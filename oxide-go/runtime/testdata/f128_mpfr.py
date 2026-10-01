#!/usr/bin/env python3
"""Regenerate the independent binary128 math corpus (requires MPFR/GMP and cc)."""
import argparse
from decimal import Decimal
from fractions import Fraction
import gzip
from pathlib import Path
import random
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
SIGN = 1 << 127
INF = 0x7fff << 112
OPS = "exp exp2 expm1 log log2 log10 log1p sin cos tan asin acos atan sinh cosh tanh asinh acosh atanh cbrt erf erfc gamma lgamma pow atan2 hypot".split()


def bits(value):
    value = Fraction(value)
    sign = SIGN if value < 0 else 0
    value = abs(value)
    if not value:
        return sign
    n, d = value.numerator, value.denominator
    e = n.bit_length() - d.bit_length()
    if value < (Fraction(2) ** e):
        e -= 1
    e = max(e, -16382)
    shift = 112 - e
    if shift >= 0:
        n <<= shift
    else:
        d <<= -shift
    q, r = divmod(n, d)
    if 2*r > d or 2*r == d and q & 1:
        q += 1
    if q >= 1 << 113:
        q >>= 1
        e += 1
    if e > 16383:
        return sign | INF
    exponent = e + 16383 if q >= 1 << 112 else 0
    return sign | exponent << 112 | (q & ((1 << 112) - 1))


def inputs():
    rng = random.Random(0x12898389)
    values = {0, SIGN, 1, SIGN | 1, (1 << 112) - 1, 1 << 112,
              INF - 1, SIGN | INF - 1, INF, SIGN | INF, INF | 1, INF | (1 << 111)}
    points = ["0.1", "0.25", "0.5", "0.75", "1", "1.5", "2", "2.4", "3", "4",
              "8", "12", "16", "32", "64", "128", "149", "150", "151", "1024", "1755", "1756", "26301.1", "26301.9",
              "11356.523406294143949491931077970765", "11433.462743336297878837243843452623",
              "3.141592653589793238462643383279502884197169399375105820974944592307816406",
              "1.570796326794896619231321691639751442098584699687552910487472296153908203"]
    for point in points:
        v = bits(Decimal(point))
        for delta in (-1, 0, 1):
            values.add(v + delta)
            values.add(SIGN | (v + delta))
    for exponent in (-16494, -16382, -114, -113, -112, -64, -16, -4, 0, 4, 16,
                     52, 53, 54, 63, 64, 112, 113, 200, 1000, 10000, 16383):
        value = bits(Fraction(2) ** exponent)
        for delta in (0, 1, 0x123456789):
            if value + delta < INF:
                values.add(value + delta)
                values.add(SIGN | (value + delta))
    for _ in range(80):
        values.add(rng.randrange(INF) | rng.randrange(2) << 127)
    for _ in range(128):
        values.add((rng.randrange(-12, 13)+16383)<<112 | rng.getrandbits(112) | rng.randrange(2)<<127)
    for op in OPS:
        if op not in ("pow", "atan2", "hypot"):
            for value in sorted(values):
                yield op, value, 0
            if op in ("sin", "cos", "tan"):
                for exponent in range(0, 16384, 61):
                    for _ in range(2):
                        yield op, (exponent+16383)<<112 | rng.getrandbits(112) | rng.randrange(2)<<127, 0
            continue
        special = [0, SIGN, bits(1), bits(-1), INF, SIGN | INF, INF | (1 << 111)]
        for x in special:
            for y in special:
                yield op, x, y
        ordered = sorted(values)
        for x in ordered:
            yield op, x, rng.choice(ordered)
        for n in (bits(2**63), bits(2**112), bits(2**113)):
            for x in (bits(1)-1, bits(1)+1, bits(1)+2):
                for sx in (0, SIGN):
                    for sy in (0, SIGN):
                        yield op, x | sx, n | sy


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=HERE / "f128_math.txt.gz")
    args = parser.parse_args()
    records = []
    mask = (1 << 64) - 1
    for op, x, y in inputs():
        records.append(f"{op} {x >> 64:016x} {x & mask:016x} {y >> 64:016x} {y & mask:016x}\n")
    with tempfile.TemporaryDirectory() as directory:
        oracle = Path(directory) / "mpfr"
        subprocess.run(["cc", "-O2", str(HERE / "f128_mpfr.c"), "-lmpfr", "-lgmp", "-o", str(oracle)], check=True)
        result = subprocess.run([str(oracle)], input="".join(records).encode(), stdout=subprocess.PIPE, check=True).stdout
    assert len(result.splitlines()) == len(records)
    args.output.write_bytes(gzip.compress(result, mtime=0))
    print(f"{len(records)} MPFR records -> {args.output}")


if __name__ == "__main__":
    main()
