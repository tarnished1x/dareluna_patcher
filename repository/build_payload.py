#!/usr/bin/env python3
"""Build encrypted source-dependent patch payloads from two mod ZIP archives.

Requires cryptography: pip install cryptography
The shipped Windows installer needs neither Python nor external tools.
"""
import argparse
import hashlib
import json
import os
import struct
import zlib
import zipfile
from pathlib import Path
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

KEY_DOMAIN = b'RibbonHeartsRivals/patch-key/v1\x00'
AAD_DOMAIN = b'RibbonHeartsRivals/payload/v1\x00'
EXTS = ('.pak', '.ucas', '.utoc')
MAGIC = b'RVP1'
BLOCK = 32
STRIDE = 8


def source_key(data: bytes) -> bytes:
    return hashlib.sha256(KEY_DOMAIN + data).digest()


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def make_delta(source: bytes, target: bytes):
    """A true copy/literal delta. Does not include complete original mod files."""
    index = {}
    if len(source) >= BLOCK:
        for pos in range(0, len(source) - BLOCK + 1, STRIDE):
            k = zlib.crc32(source[pos:pos + BLOCK])
            # For collisions prefer the earlier entry. Confirm bytes on match.
            if k not in index:
                index[k] = pos
    chunks = [MAGIC, struct.pack('<Q', len(target))]
    i = 0
    literal_start = 0
    copy_bytes = 0
    copies = 0
    while i < len(target):
        offset = -1
        if len(target) - i >= BLOCK:
            k = zlib.crc32(target[i:i + BLOCK])
            candidate = index.get(k, -1)
            if candidate >= 0 and source[candidate:candidate + BLOCK] == target[i:i + BLOCK]:
                offset = candidate
        if offset < 0:
            i += 1
            if i - literal_start >= 1024 * 1024:
                literal = target[literal_start:i]
                chunks.extend((b'L', struct.pack('<I', len(literal)), literal))
                literal_start = i
            continue
        if i > literal_start:
            literal = target[literal_start:i]
            chunks.extend((b'L', struct.pack('<I', len(literal)), literal))
        end = i + BLOCK
        while end < len(target) and offset + end - i < len(source) and target[end] == source[offset + end - i]:
            end += 1
        length = end - i
        chunks.extend((b'C', struct.pack('<QI', offset, length)))
        copies += 1
        copy_bytes += length
        i = end
        literal_start = i
    if literal_start < len(target):
        literal = target[literal_start:]
        chunks.extend((b'L', struct.pack('<I', len(literal)), literal))
    chunks.append(b'E')
    return b''.join(chunks), copy_bytes, copies


def read_unique(z, ext):
    entries = [i for i in z.infolist() if not i.is_dir() and i.filename.lower().endswith(ext)]
    if len(entries) != 1:
        raise ValueError(f'Expected exactly one {ext} in {z.filename}, got {len(entries)}')
    entry = entries[0]
    if '/' in entry.filename or '\\' in entry.filename:
        raise ValueError(f'Archive entry must be at archive root: {entry.filename!r}')
    return entry.filename, z.read(entry)


def apply_test(src, payload, output_name):
    assert payload[:4] == b'RVE1'
    plain = AESGCM(source_key(src)).decrypt(payload[4:16], payload[16:], AAD_DOMAIN + output_name.encode('utf-8'))
    uncompressed = zlib.decompress(plain)
    assert uncompressed[:4] == MAGIC
    expected = struct.unpack_from('<Q', uncompressed, 4)[0]
    i = 12
    result = bytearray()
    while True:
        kind = uncompressed[i:i+1]; i+=1
        if kind == b'E': break
        if kind == b'C':
            offset, length = struct.unpack_from('<QI', uncompressed, i); i+=12
            result.extend(src[offset:offset+length])
        elif kind == b'L':
            length = struct.unpack_from('<I', uncompressed, i)[0]; i+=4
            result.extend(uncompressed[i:i+length]); i+=length
        else:
            raise ValueError('Unrecognized delta opcode')
    assert i == len(uncompressed), 'trailing data'
    assert expected == len(result)
    return bytes(result)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('original_zip')
    p.add_argument('modified_zip')
    p.add_argument('--out', default='payload')
    args = p.parse_args()
    output = Path(args.out); output.mkdir(parents=True, exist_ok=True)
    records = []
    with zipfile.ZipFile(args.original_zip) as originals, zipfile.ZipFile(args.modified_zip) as modified:
        for ext in EXTS:
            source_name, source_data = read_unique(originals, ext)
            output_name, target_data = read_unique(modified, ext)
            delta, copied, copies = make_delta(source_data, target_data)
            compressed = zlib.compress(delta, 9)
            nonce = os.urandom(12)
            ciphertext = AESGCM(source_key(source_data)).encrypt(
                nonce, compressed, AAD_DOMAIN + output_name.encode('utf-8'))
            payload = b'RVE1' + nonce + ciphertext
            payload_name = f'{ext[1:]}.rvp'
            (output / payload_name).write_bytes(payload)
            if apply_test(source_data, payload, output_name) != target_data:
                raise ValueError(f'Patch round-trip failed: {ext}')
            record = {
                'source': source_name,
                'output': output_name,
                'source_size': len(source_data),
                'output_size': len(target_data),
                'source_sha256': sha(source_data),
                'output_sha256': sha(target_data),
                'payload': payload_name,
                'payload_sha256': sha(payload),
            }
            records.append(record)
            print(f'{ext}: {len(source_data):,} -> {len(target_data):,} bytes; '
                  f'copy {copied:,} bytes ({copies} blocks), payload {len(payload):,} bytes')
    manifest = {'format_version': 1, 'game': 'Marvel Rivals',
                'title': 'dareluna patcher', 'files': records}
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
    print('Payload verified and written to:', output)


if __name__ == '__main__':
    main()
