#!/usr/bin/env python3
"""Generate assets/fanfare-default.wav: a short brass-like fanfare, synthesized from scratch.

Python 3 standard library only (wave, math, struct). Deterministic: no randomness, so the same bytes
come out on every run. Mono, 22050 Hz, 16-bit, about 3.6 s.

Usage: python3 scripts/gen_fanfare_default.py [OUT]   (default: assets/fanfare-default.wav)
"""
import math
import os
import struct
import sys
import wave

RATE = 22050

# Note name -> frequency (equal temperament, A4 = 440 Hz).
def hz(semitones_from_a4):
    return 440.0 * 2 ** (semitones_from_a4 / 12.0)

G4, C5, E5, G5, C6 = hz(-2), hz(3), hz(7), hz(10), hz(15)

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
        f = freq * (1.0 + vib * math.sin(2 * math.pi * 5.2 * t) * min(1.0, t / 0.5))
        phase += 2 * math.pi * f / RATE
        bright = 0.35 + 0.65 * env  # fewer overtones when the note is quiet, like a real horn
        s = 0.0
        for k, h in enumerate(HARMONICS, start=1):
            if f * k >= RATE / 2:
                break
            s += h * (bright ** (k - 1)) * math.sin(k * phase)
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
    pcm = b"".join(struct.pack("<h", int(round(v * scale))) for v in buf)
    with wave.open(out, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(RATE)
        w.writeframes(pcm)


if __name__ == "__main__":
    main()
