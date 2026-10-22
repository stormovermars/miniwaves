#!/usr/bin/env python3
"""Generate assets/fanfare-default.wav: a short brass-like fanfare, synthesized from scratch.

Python 3 standard library only (wave, math, struct). Deterministic: no randomness, so the same bytes
come out on every run. Mono, 22050 Hz, 16-bit, about 3.6 s.

Byte-identical on Linux, macOS and Windows (CI compares the sha256 against the shipped WAV on all three, TSK-061):
the maths library (libm / the Windows CRT) may round sin() and pow() differently, so this script uses neither.
It calls only + - * / floor on IEEE doubles, which every platform rounds the same way, and the note frequencies
are literals. The final rounding to 16 bits is floor(x + 0.5), not round().

Usage: python3 scripts/gen_fanfare_default.py [OUT]   (default: assets/fanfare-default.wav)
"""
import math
import os
import struct
import sys
import wave

RATE = 22050

# Note frequencies in Hz (equal temperament, A4 = 440 Hz: 440 * 2 ** (n / 12) for n = -2, 3, 7, 10, 15), written
# out as literals so no pow() runs here.
G4, C5, E5, G5, C6 = 391.99543598174927, 523.2511306011972, 659.2551138257398, 783.9908719634985, 1046.5022612023945

TWO_PI = 2.0 * math.pi
PI = math.pi
HALF_PI = math.pi / 2.0
# sin x = x * (1 + x^2 * (c1 + c2 x^2 + ...)) with c_j = (-1)^j / (2j+1)!, built by exact-rounded division (no libm).
_SIN_C = []
_f = 1.0
for _j in range(1, 11):
    _f *= (2 * _j) * (2 * _j + 1)
    _SIN_C.append((-1.0 if _j % 2 else 1.0) / _f)


def sin(x):
    """sin(x) from + - * / floor only: reduce to [-pi/2, pi/2], then a Taylor polynomial (error below 1e-16)."""
    x = x - TWO_PI * math.floor(x / TWO_PI)
    if x > PI:
        x -= TWO_PI
    if x > HALF_PI:
        x = PI - x
    elif x < -HALF_PI:
        x = -PI - x
    x2 = x * x
    acc = 0.0
    for c in reversed(_SIN_C):
        acc = acc * x2 + c
    return x * (1.0 + x2 * acc)


# Relative strengths of harmonics 1..10: a brass spectrum is rich and falls off slowly.
HARMONICS = [1.0, 0.85, 0.7, 0.55, 0.42, 0.3, 0.2, 0.13, 0.08, 0.05]


def brass_amp(t, dur, attack=0.045, release=0.18):
    """Envelope: quick swell, slight sag, sustain, soft release."""
    if t < attack:
        a = t / attack
        return a * a * (3 - 2 * a)  # smoothstep swell
    amp = 1.0 - 0.12 * min(1.0, (t - attack) / 0.4)
    left = dur - t
    if left < release:
        r = max(0.0, left / release)
        amp *= r * r
    return amp


def tone(freq, dur, vib=0.0):
    """One brass note as a list of floats in [-1, 1]; brightness follows the envelope (swell opens the tone)."""
    n = int(dur * RATE)
    out = []
    phase = 0.0
    for i in range(n):
        t = i / RATE
        env = brass_amp(t, dur)
        # vibrato fades in on long notes
        f = freq * (1.0 + vib * sin(2 * math.pi * 5.2 * t) * min(1.0, t / 0.5))
        phase += 2 * math.pi * f / RATE
        if phase >= TWO_PI:  # keep the phase small: sin() is then accurate to the last bit or so
            phase -= TWO_PI
        bright = 0.35 + 0.65 * env  # fewer overtones when the note is quiet, like a real horn
        s = 0.0
        bk = 1.0  # bright ** (k - 1) by repeated multiplication, not pow()
        for k, h in enumerate(HARMONICS, start=1):
            if f * k >= RATE / 2:
                break
            s += h * bk * sin(k * phase)
            bk *= bright
        out.append(s * env)
    return out


def mix(total, parts):
    buf = [0.0] * int(total * RATE)
    for start, samples, gain in parts:
        o = int(start * RATE)
        for i, v in enumerate(samples):
            if o + i < len(buf):
                buf[o + i] += v * gain
    return buf


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "assets", "fanfare-default.wav")
    # "ta-ta-ta TAAA, ta-TAAAAA": pickup triplet, a rising call, then a held C major chord.
    parts = [
        (0.00, tone(G4, 0.24), 0.9),
        (0.25, tone(C5, 0.24), 0.9),
        (0.50, tone(E5, 0.24), 0.9),
        (0.75, tone(G5, 0.62), 1.0),
        (1.45, tone(E5, 0.22), 0.9),
        (1.70, tone(G5, 0.22), 0.95),
    ]
    chord = 1.95
    for f, g in ((C5, 0.8), (E5, 0.75), (G5, 0.8), (C6, 0.7)):
        parts.append((chord, tone(f, 1.65, vib=0.004), g))
    buf = mix(3.65, parts)
    peak = max(abs(v) for v in buf) or 1.0
    scale = 0.85 * 32767 / peak
    pcm = b"".join(struct.pack("<h", int(math.floor(v * scale + 0.5))) for v in buf)
    with wave.open(out, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(RATE)
        w.writeframes(pcm)


if __name__ == "__main__":
    main()
