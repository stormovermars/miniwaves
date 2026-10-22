"""Writes alert.wav next to this script: two sine tones, generated, not sampled."""
import math, os, struct, wave

rate = 22050


def tone(freq, ms, vol=0.5):
    n = int(rate * ms / 1000)
    out = []
    for i in range(n):
        env = min(1, i / (rate * 0.01)) * min(1, (n - i) / (rate * 0.06))
        out.append(int(32767 * vol * env * math.sin(2 * math.pi * freq * i / rate)))
    return out


data = tone(880, 140) + [0] * int(rate * 0.03) + tone(1320, 260)
path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "alert.wav")
with wave.open(path, "wb") as w:
    w.setnchannels(1)
    w.setsampwidth(2)
    w.setframerate(rate)
    w.writeframes(b"".join(struct.pack("<h", x) for x in data))
