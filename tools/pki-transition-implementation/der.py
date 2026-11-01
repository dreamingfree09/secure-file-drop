"""Bounded strict DER reader for the deliberately narrow fixture profile, not general ASN.1."""
import datetime
from fence import LIMIT, Stop

def nodes(data):
    if not isinstance(data, bytes) or len(data) > LIMIT:
        raise Stop("DER-size")
    out, i = [], 0
    while i < len(data):
        start = i
        tag = data[i]
        i += 1
        if tag & 31 == 31 or i == len(data):
            raise Stop("DER-tag")
        n = data[i]
        i += 1
        if n & 128:
            count = n & 127
            if not 1 <= count <= 4 or i + count > len(data) or data[i] == 0:
                raise Stop("DER-length")
            n = int.from_bytes(data[i:i+count], "big")
            i += count
            if n < 128:
                raise Stop("DER-nonminimal-length")
        if i + n > len(data):
            raise Stop("DER-truncated")
        out.append((tag, data[i:i+n], data[start:i+n]))
        i += n
        if len(out) > 10000:
            raise Stop("DER-count")
    return out

def one(data, tag):
    v = nodes(data)
    if len(v) != 1 or v[0][0] != tag:
        raise Stop("DER-wrapper")
    return v[0][1]

def seq(data):
    return nodes(one(data, 0x30))

def integer(n):
    tag, v, _ = n
    if tag != 2 or not v or v[0] & 128 or (len(v) > 1 and v[0] == 0 and not v[1] & 128):
        raise Stop("DER-integer")
    return int.from_bytes(v, "big")

def exact_integer(data):
    n = nodes(data)
    if len(n) != 1:
        raise Stop("DER-integer-wrapper")
    return integer(n[0])

def oid(n):
    if n[0] != 6:
        raise Stop("DER-OID")
    return n[1].hex()

def utc(n):
    if n[0] not in (23,24):
        raise Stop("DER-time-tag")
    v = n[1].decode("ascii")
    fmt = "%y%m%d%H%M%SZ" if n[0] == 23 else "%Y%m%d%H%M%SZ"
    if len(v) != (13 if n[0] == 23 else 15):
        raise Stop("DER-time-length")
    d = datetime.datetime.strptime(v, fmt).replace(tzinfo=datetime.timezone.utc)
    if n[0] == 23 and d.year > 2049:
        d = d.replace(year=d.year-100)
    return int(d.timestamp())

def extensions(data):
    result = {}
    for x in seq(data):
        fields = seq(x[2])
        if len(fields) not in (2,3):
            raise Stop("DER-extension")
        name = oid(fields[0])
        if name in result:
            raise Stop("DER-duplicate-extension")
        critical = len(fields) == 3
        if critical and fields[1] != (1, b"\xff", b"\x01\x01\xff"):
            raise Stop("DER-critical")
        value = fields[-1]
        if value[0] != 4:
            raise Stop("DER-ext-value")
        result[name] = (critical, value[1])
    return result

SHA256_RSA = bytes.fromhex("300d06092a864886f70d01010b0500")
SKI, AKI, BASIC, KU, EKU, SAN, CDP, NUMBER = (
    "551d0e","551d23","551d13","551d0f","551d25","551d11","551d1f","551d14")

def signed(data):
    p = seq(data)
    if len(p) != 3 or p[0][0] != 0x30 or p[1][2] != SHA256_RSA or p[2][0] != 3 or p[2][1][:1] != b"\0":
        raise Stop("signature-profile")
    return p[0]

def certificate(data):
    tbs = signed(data)
    p = nodes(tbs[1])
    if len(p) != 8 or p[0][0] != 0xa0 or exact_integer(p[0][1]) != 2:
        raise Stop("certificate-profile")
    if p[2][2] != SHA256_RSA or p[7][0] != 0xa3:
        raise Stop("certificate-algorithm-extensions")
    serial = integer(p[1])
    if not 0 < serial < 2**160 or len(p[1][1]) > 20:
        raise Stop("certificate-serial")
    dates = seq(p[4][2])
    if len(dates) != 2:
        raise Stop("certificate-validity")
    ext = extensions(p[7][1])
    spki = seq(p[6][2])
    if len(spki) != 2 or spki[0][2] != bytes.fromhex("300d06092a864886f70d0101010500") or spki[1][0] != 3 or spki[1][1][0] != 0:
        raise Stop("certificate-SPKI")
    rsa = seq(spki[1][1][1:])
    if len(rsa) != 2 or integer(rsa[0]).bit_length() != 3072 or integer(rsa[1]) != 65537:
        raise Stop("certificate-RSA")
    return {"issuer":p[3][2], "subject":p[5][2], "serial":serial,
            "not_before":utc(dates[0]), "not_after":utc(dates[1]), "ext":ext,
            "spki":p[6][2]}

def authority(ext):
    fields = seq(ext[AKI][1])
    if len(fields) != 1 or fields[0][0] != 0x80 or len(fields[0][1]) != 20:
        raise Stop("AKI-profile")
    return fields[0][1]

def crl(data):
    p = nodes(signed(data)[1])
    if len(p) not in (6,7) or integer(p[0]) != 1 or p[1][2] != SHA256_RSA:
        raise Stop("CRL-profile")
    if p[2][0] != 0x30 or p[-1][0] != 0xa0:
        raise Stop("CRL-issuer-extensions")
    ext = extensions(p[-1][1])
    if set(ext) != {AKI,NUMBER} or any(x[0] for x in ext.values()):
        raise Stop("CRL-extension-profile")
    number = exact_integer(ext[NUMBER][1])
    if not 0x2000 <= number < 2**159:
        raise Stop("CRL-sequence")
    revoked = set()
    if len(p) == 7:
        if p[5][0] != 0x30:
            raise Stop("CRL-revoked-sequence")
        for x in nodes(p[5][1]):
            fields = seq(x[2])
            if len(fields) != 2:
                raise Stop("CRL-revocation-entry")
            serial = integer(fields[0])
            if not 0 < serial < 2**159 or utc(fields[1]) > utc(p[3]):
                raise Stop("CRL-revoked-entry-profile")
            if serial in revoked:
                raise Stop("CRL-duplicate-serial")
            revoked.add(serial)
    return {"issuer":p[2][2], "start":utc(p[3]), "end":utc(p[4]),
            "aki":authority(ext), "number":number, "revoked":revoked}

def profiles(root, leaf, cdp, now):
    r, l = certificate(root), certificate(leaf)
    # Only obviously synthetic issuers are accepted by this preparation implementation.
    if b"SyntheticSFDFixture-" not in r["subject"] or r["issuer"] != r["subject"]:
        raise Stop("synthetic-issuer-required")
    e = r["ext"]
    if set(e) != {SKI,AKI,BASIC,KU} or e[BASIC] != (True, bytes.fromhex("30060101ff020100")) or e[KU] != (True, bytes.fromhex("03020106")):
        raise Stop("root-extensions")
    if e[SKI][0] or e[AKI][0]:
        raise Stop("root-extension-criticality")
    ski = one(e[SKI][1],4)
    if len(ski) != 20 or authority(e) != ski:
        raise Stop("root-identifiers")
    e = l["ext"]
    if set(e) != {SKI,AKI,BASIC,KU,EKU,SAN,CDP} or e[BASIC] != (True,b"\x30\0") or e[KU] != (True,bytes.fromhex("030205a0")):
        raise Stop("leaf-extensions")
    if e[EKU] != (False,bytes.fromhex("300a06082b06010505070301")) or authority(e) != ski or l["issuer"] != r["subject"]:
        raise Stop("leaf-usage-issuer")
    if any(e[x][0] for x in (SKI,AKI,EKU,SAN,CDP)):
        raise Stop("leaf-extension-criticality")
    names = nodes(one(e[SAN][1],0x30))
    expected = {(0x82,b"localhost"),(0x87,b"\x7f\0\0\x01"),(0x87,b"\0"*15+b"\x01")}
    if {(x[0],x[1]) for x in names} != expected or len(names) != 3:
        raise Stop("leaf-SAN")
    # Full CDP encoding: one DistributionPoint/fullName/one URI.
    dp = seq(e[CDP][1])
    if len(dp) != 1:
        raise Stop("leaf-CDP-count")
    fields = seq(dp[0][2])
    if len(fields) != 1 or fields[0][0] != 0xa0:
        raise Stop("leaf-CDP")
    uri = nodes(one(fields[0][1],0xa0))
    if len(uri) != 1 or uri[0][0] != 0x86 or uri[0][1] != cdp.encode("ascii"):
        raise Stop("leaf-CDP")
    if not r["not_before"] <= now < r["not_after"] or r["not_after"] - r["not_before"] > 730*86400:
        raise Stop("root-validity")
    if not l["not_before"] <= now < l["not_after"] or l["not_after"] - l["not_before"] > 90*86400 or l["not_after"] > r["not_after"]:
        raise Stop("leaf-validity")
    return r,l,ski
