package main

import (
    "bufio"
    "bytes"
    "compress/zlib"
    "crypto/aes"
    "crypto/cipher"
    "crypto/sha256"
    "embed"
    "encoding/binary"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

//go:embed payload/*
var patchFiles embed.FS

var keyDomain = []byte("RibbonHeartsRivals/patch-key/v1\x00")
var aadDomain = []byte("RibbonHeartsRivals/payload/v1\x00")

type fileRecord struct {
    Source string `json:"source"`
    Output string `json:"output"`
    SourceSize int64 `json:"source_size"`
    OutputSize int64 `json:"output_size"`
    SourceSHA string `json:"source_sha256"`
    OutputSHA string `json:"output_sha256"`
    Payload string `json:"payload"`
    PayloadSHA string `json:"payload_sha256"`
}

type patchManifest struct {
    FormatVersion int `json:"format_version"`
    Game string `json:"game"`
    Title string `json:"title"`
    Files []fileRecord `json:"files"`
}

type preparedFile struct {
    target string
    temp string
}

type installResult struct { Created, Existing int }

func shaHex(data []byte) string {
    h := sha256.Sum256(data)
    return hex.EncodeToString(h[:])
}

func validFilename(name string) bool {
    if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
        return false
    }
    if strings.ContainsAny(name, "/\\:\x00") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
        return false
    }
    return true
}

func loadManifest() (patchManifest, error) {
    var manifest patchManifest
    content, err := patchFiles.ReadFile("payload/manifest.json")
    if err != nil {return manifest, err}
    if err = json.Unmarshal(content, &manifest); err != nil {return manifest, err}
    if manifest.FormatVersion != 1 || manifest.Game != "Marvel Rivals" || len(manifest.Files) != 3 {
        return manifest, errors.New("Unsupported or incomplete patch manifest")
    }
    seen := map[string]bool{}
    for _, r := range manifest.Files {
        if !validFilename(r.Source) || !validFilename(r.Output) || !validFilename(r.Payload) {
            return manifest, errors.New("Patch manifest contains an unsafe filename")
        }
        if r.SourceSize < 1 || r.OutputSize < 1 || r.OutputSize > 1<<30 {
            return manifest, errors.New("Invalid file size in patch manifest")
        }
        ext := strings.ToLower(filepath.Ext(r.Output))
        if ext != ".pak" && ext != ".ucas" && ext != ".utoc" {
            return manifest, errors.New("Unsupported output extension")
        }
        if seen[ext] { return manifest, errors.New("Duplicate output extension") }
        seen[ext] = true
        for _, digest := range []string{r.SourceSHA, r.OutputSHA, r.PayloadSHA} {
            b, e := hex.DecodeString(digest)
            if e != nil || len(b) != sha256.Size { return manifest, errors.New("Invalid manifest digest") }
        }
    }
    return manifest, nil
}

func fileSHA(path string) (string, error) {
    fd, err := os.Open(path)
    if err != nil {return "", err}
    defer fd.Close()
    h := sha256.New()
    if _, err = io.Copy(h, fd); err != nil {return "", err}
    return hex.EncodeToString(h.Sum(nil)), nil
}

func deriveKey(srcFile string) ([]byte, error) {
    fd, err := os.Open(srcFile)
    if err != nil {return nil, err}
    defer fd.Close()
    h := sha256.New()
    h.Write(keyDomain)
    if _, err = io.Copy(h, fd); err != nil {return nil, err}
    return h.Sum(nil), nil
}

func applyEncryptedPatch(originalFile string, record fileRecord, tempFile string) error {
    encrypted, err := patchFiles.ReadFile("payload/" + record.Payload)
    if err != nil {return fmt.Errorf("patch payload not found: %w", err)}
    if shaHex(encrypted) != record.PayloadSHA {return errors.New("Embedded patch integrity check failed")}
    if len(encrypted) <= 16 || !bytes.Equal(encrypted[:4], []byte("RVE1")) {
        return errors.New("Invalid encrypted patch header")
    }
    key, err := deriveKey(originalFile)
    if err != nil {return err}
    block, err := aes.NewCipher(key)
    if err != nil {return err}
    gcm, err := cipher.NewGCM(block)
    if err != nil {return err}
    aad := append(append([]byte{}, aadDomain...), []byte(record.Output)...)
    plain, err := gcm.Open(nil, encrypted[4:16], encrypted[16:], aad)
    if err != nil {return errors.New("Cannot unlock patch: original file does not match")}
    zr, err := zlib.NewReader(bytes.NewReader(plain))
    if err != nil {return err}
    defer zr.Close()
    reader := bufio.NewReaderSize(zr, 65536)
    magic := make([]byte, 4)
    if _, err = io.ReadFull(reader, magic); err != nil {return err}
    if !bytes.Equal(magic, []byte("RVP1")) {return errors.New("Unsupported delta format")}
    var outputSize uint64
    if err = binary.Read(reader, binary.LittleEndian, &outputSize); err != nil {return err}
    if outputSize != uint64(record.OutputSize) {return errors.New("Delta output size is incorrect")}

    original, err := os.Open(originalFile)
    if err != nil {return err}
    defer original.Close()
    stat, err := original.Stat()
    if err != nil {return err}
    out, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_TRUNC, 0600)
    if err != nil {return err}
    defer out.Close()
    hasher := sha256.New()
    writer := io.MultiWriter(out, hasher)
    var written int64
    for {
        op, err := reader.ReadByte()
        if err != nil {return errors.New("Patch ended unexpectedly")}
        if op == 'E' {break}
        var length uint32
        if op == 'L' {
            if err = binary.Read(reader, binary.LittleEndian, &length); err != nil {return err}
            if length == 0 || written+int64(length) > record.OutputSize {return errors.New("Invalid literal length")}
            n, e := io.CopyN(writer, reader, int64(length))
            if e != nil || n != int64(length) {return errors.New("Truncated literal patch data")}
            written += n
        } else if op == 'C' {
            var offset uint64
            if err = binary.Read(reader, binary.LittleEndian, &offset); err != nil {return err}
            if err = binary.Read(reader, binary.LittleEndian, &length); err != nil {return err}
            if length == 0 || offset > uint64(stat.Size()) || uint64(length) > uint64(stat.Size())-offset || written+int64(length) > record.OutputSize {
                return errors.New("Invalid copy instruction in patch")
            }
            n, e := io.CopyN(writer, io.NewSectionReader(original, int64(offset), int64(length)), int64(length))
            if e != nil || n != int64(length) {return errors.New("Cannot read original file segment")}
            written += n
        } else {
            return errors.New("Unknown patch instruction")
        }
    }
    if written != record.OutputSize {return errors.New("Patch produced the wrong file size")}
    if _, err := reader.ReadByte(); err != io.EOF {return errors.New("Unexpected trailing patch bytes")}
    if hex.EncodeToString(hasher.Sum(nil)) != record.OutputSHA {
        return errors.New("Patched file failed SHA-256 verification")
    }
    if err = out.Sync(); err != nil {return err}
    return out.Close()
}

func InstallPatch(originalDir, outputDir string, status func(string, int)) (installResult, error) {
    report := installResult{}
    notify := func(msg string, progress int) { if status != nil {status(msg, progress)} }
    originalDir = strings.TrimSpace(strings.Trim(originalDir, `"`))
    outputDir = strings.TrimSpace(strings.Trim(outputDir, `"`))
    if originalDir == "" || outputDir == "" {return report, errors.New("Choose both the original mod folder and the installation folder")}
    sources, err := filepath.Abs(originalDir)
    if err != nil {return report, err}
    dest, err := filepath.Abs(outputDir)
    if err != nil {return report, err}
    if sourceInfo, err := os.Stat(sources); err != nil || !sourceInfo.IsDir() {
        return report, errors.New("Original mod folder not found. Extract the original ZIP first")
    }
    manifest, err := loadManifest()
    if err != nil {return report, err}
    notify("Checking the three original mod files...", 8)
    for i, record := range manifest.Files {
        source := filepath.Join(sources, record.Source)
        info, e := os.Stat(source)
        if e != nil || !info.Mode().IsRegular() {return report, fmt.Errorf("Missing original file:\n%s\n\nExtract the original ZIP and select its folder", record.Source)}
        if info.Size() != record.SourceSize {return report, fmt.Errorf("Original file is the wrong version:\n%s", record.Source)}
        h, e := fileSHA(source)
        if e != nil {return report, fmt.Errorf("Cannot verify %s: %w", record.Source, e)}
        if h != record.SourceSHA {return report, fmt.Errorf("Original file is the wrong version:\n%s", record.Source)}
        notify(fmt.Sprintf("Verified original %d of 3...", i+1), 12+(i+1)*10)
    }
    if fi, e := os.Stat(dest); e == nil && !fi.IsDir() {
        return report, errors.New("The installation destination is not a folder")
    } else if e != nil && !os.IsNotExist(e) {return report, e}
    // Preflight every destination before producing anything. Never overwrite.
    toMake := []fileRecord{}
    for _, record := range manifest.Files {
        output := filepath.Join(dest, record.Output)
        if fi, e := os.Lstat(output); e == nil {
            if !fi.Mode().IsRegular() {return report, fmt.Errorf("Destination is not a regular file: %s", record.Output)}
            if fi.Size() == record.OutputSize {
                existingHash, hashErr := fileSHA(output)
                if hashErr == nil && existingHash == record.OutputSHA {report.Existing++; continue}
            }
            return report, fmt.Errorf("An existing file would be overwritten:\n%s\n\nMove or back up that file first", record.Output)
        } else if !os.IsNotExist(e) {return report, e}
        toMake = append(toMake, record)
    }
    if len(toMake) == 0 { notify("Everything is already installed! ♡", 100); return report, nil }
    if err = os.MkdirAll(dest, 0755); err != nil {return report, fmt.Errorf("Cannot create installation folder: %w", err)}
    staged := []preparedFile{}
    defer func() { for _, item := range staged { if item.temp != "" {os.Remove(item.temp)} } }()
    for i, record := range toMake {
        notify(fmt.Sprintf("Creating %s...", record.Output), 46+(i*42)/len(toMake))
        tmp, e := os.CreateTemp(dest, ".ribbon-hearts-*.tmp")
        if e != nil {return report, fmt.Errorf("Cannot write to installation folder: %w", e)}
        tempName := tmp.Name()
        tmp.Close()
        staged = append(staged, preparedFile{target: filepath.Join(dest, record.Output), temp: tempName})
        if e = applyEncryptedPatch(filepath.Join(sources, record.Source), record, tempName); e != nil {
            return report, fmt.Errorf("Could not create %s: %w", record.Output, e)
        }
        notify(fmt.Sprintf("Verified %s ♡", record.Output), 46+((i+1)*42)/len(toMake))
    }
    // Commit only after all generated files have passed SHA-256 verification.
    added := []string{}
    for _, item := range staged {
        if _, e := os.Lstat(item.target); e == nil {
            for _, path := range added {os.Remove(path)}
            return report, fmt.Errorf("A target file appeared during installation: %s", filepath.Base(item.target))
        } else if !os.IsNotExist(e) {
            for _, path := range added {os.Remove(path)}
            return report, e
        }
        if e := os.Rename(item.temp, item.target); e != nil {
            for _, path := range added {os.Remove(path)}
            return report, fmt.Errorf("Could not finish installation: %w", e)
        }
        added = append(added, item.target)
        report.Created++
    }
    notify("Installation complete! ♥", 100)
    return report, nil
}
