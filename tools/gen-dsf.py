#!/usr/bin/env python3
"""gen-dsf.py — synthesize a small DSF (DSD) test file without any external library.

    python gen-dsf.py out.dsf [--rate 64|128|256] [--seconds 3] [--freq 1000] [--level 0.5]

A first-order sigma-delta modulator turns a sine into a 1-bit stream at 44100*rate*64 Hz.
Audio quality is only "test tone" grade (first-order noise shaping), but it is a valid DSF
that MPD, foobar2000 and most DACs accept, so it proves whether a DAC path is native DSD, DoP
or software conversion (see deploy/scripts/test-audio.sh --dsd).
"""
import argparse, math, struct, sys

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("out"); ap.add_argument("--rate", type=int, default=64, choices=[64, 128, 256])
    ap.add_argument("--seconds", type=float, default=3.0); ap.add_argument("--freq", type=float, default=1000.0)
    ap.add_argument("--level", type=float, default=0.5)
    a = ap.parse_args()

    fs = 44100 * a.rate
    n = int(fs * a.seconds)
    block = 4096
    nbytes = (n + 7) // 8
    padded = (nbytes + block - 1) // block * block

    # first-order sigma-delta, LSB-first bit packing (DSF bits_per_sample = 1)
    buf = bytearray(padded)
    integ = 0.0; prev = 0.0
    w = 2 * math.pi * a.freq / fs; lvl = a.level
    byte = 0; bit = 0; i = 0
    sin = math.sin
    for k in range(n):
        x = lvl * sin(w * k)
        integ += x - prev
        y = 1.0 if integ >= 0.0 else -1.0
        prev = y
        if y > 0: byte |= (1 << bit)
        bit += 1
        if bit == 8:
            buf[i] = byte; i += 1; byte = 0; bit = 0
    if bit: buf[i] = byte

    ch = 2
    data_size = padded * ch
    with open(a.out, "wb") as f:
        total = 28 + 52 + 12 + data_size
        f.write(b"DSD " + struct.pack("<QQQ", 28, total, 0))
        f.write(b"fmt " + struct.pack("<QIIIIIIQII", 52, 1, 0, 2, ch, fs, 1, n, block, 0))
        f.write(b"data" + struct.pack("<Q", 12 + data_size))
        for off in range(0, padded, block):
            chunk = bytes(buf[off:off + block])
            for _ in range(ch): f.write(chunk)
    print(f"wrote {a.out}: DSD{a.rate} ({fs} Hz), {a.seconds}s, {ch} ch, {total/1e6:.1f} MB")

if __name__ == "__main__":
    sys.exit(main())
