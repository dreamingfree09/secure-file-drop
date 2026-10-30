"""Synthetic-only 32 MiB ext4-in-VHDX fixtures, MS-VHDX/QEMU layouts.
Not a decoder, validator or repair implementation for Owner images.
"""
from pathlib import Path
import struct
from uuid import UUID

MIB = 1024 * 1024
RAW_SIZE = 32 * MIB

def crc32c(data):
    value = 0xffffffff
    for byte in data:
        value ^= byte
        for _ in range(8):
            value = (value >> 1) ^ (0x82f63b78 if value & 1 else 0)
    return value ^ 0xffffffff

assert crc32c(b"123456789") == 0xe3069283

def guid(value):
    return UUID(value).bytes_le

def make_vhdx(path, raw, pending=True, corrupt_log=False):
    """CreateNew synthetic container; pending transaction restores its BAT."""
    assert len(raw) == RAW_SIZE and raw[1080:1082] == b"\x53\xef"
    path = Path(path)
    assert not path.exists() and not path.is_symlink()
    header = bytearray(MIB)
    header[:8] = b"vhdxfile"
    log_guid = guid("44444444-4444-4444-4444-444444444444") if pending else bytes(16)
    for offset, sequence in [(65536, 1), (131072, 2)]:
        item = bytearray(4096)
        item[:4] = b"head"
        struct.pack_into("<Q", item, 8, sequence)
        item[16:32] = guid("11111111-1111-1111-1111-111111111111")
        item[32:48] = guid("22222222-2222-2222-2222-222222222222")
        item[48:64] = log_guid
        struct.pack_into("<HHIQ", item, 64, 0, 1, MIB, MIB)
        struct.pack_into("<I", item, 4, crc32c(item))
        header[offset:offset+4096] = item
    region = bytearray(65536)
    region[:4] = b"regi"
    struct.pack_into("<I", region, 8, 2)
    region[16:32] = guid("2dc27766-f623-4200-9d64-115e9bfd4a08")
    struct.pack_into("<QII", region, 32, 2*MIB, MIB, 1)
    region[48:64] = guid("8b7ca206-4790-4b9a-b8fe-575f050f886e")
    struct.pack_into("<QII", region, 64, 3*MIB, MIB, 1)
    struct.pack_into("<I", region, 4, crc32c(region))
    header[196608:262144] = region
    header[262144:327680] = region
    bat = bytearray(MIB)
    for index in range(16):
        struct.pack_into("<Q", bat, index*8, (4*MIB+index*2*MIB)|6)
    metadata = bytearray(MIB)
    metadata[:8] = b"metadata"
    entries = [
        ("caa16737-fa36-4d43-b3b6-33f0aa44e76b", struct.pack("<II",2*MIB,1),4),
        ("2fa54224-cd1b-4876-b211-5dbed83bf4b8", struct.pack("<Q",RAW_SIZE),6),
        ("8141bf1d-a96f-4709-ba47-f233a8faab5f", struct.pack("<I",512),6),
        ("cda348c7-445d-4471-9cc9-e9885251c556", struct.pack("<I",4096),6),
        ("beca12ab-b2e6-4523-93ef-c309e000c746",guid("33333333-3333-3333-3333-333333333333"),6)]
    struct.pack_into("<H",metadata,10,len(entries))
    position = 65536
    for index,(identifier,data,flags) in enumerate(entries):
        offset=32+index*32
        metadata[offset:offset+16]=guid(identifier)
        struct.pack_into("<IIII",metadata,offset+16,position,len(data),flags,0)
        metadata[position:position+len(data)]=data
        position+=len(data)
    log=bytearray(MIB)
    if pending:
        page=bytes(bat[:4096])
        entry=bytearray(8192)
        size=36*MIB
        struct.pack_into("<4sIIIQII16sQQ",entry,0,b"loge",0,8192,0,1,1,0,log_guid,size,size)
        struct.pack_into("<4sIQQQ",entry,64,b"desc",int.from_bytes(page[4092:4096],"little"),int.from_bytes(page[:8],"little"),2*MIB,1)
        entry[4096:4100]=b"data"
        struct.pack_into("<I",entry,4100,0)
        entry[4104:8188]=page[8:4092]
        struct.pack_into("<I",entry,8188,1)
        struct.pack_into("<I",entry,4,crc32c(entry))
        if corrupt_log:
            entry[4]^=1
        log[:8192]=entry
        struct.pack_into("<Q",bat,0,2)
    with path.open("xb") as target:
        for part in [header,log,bat,metadata,raw]:
            target.write(part)
    assert path.stat().st_size == 36*MIB
