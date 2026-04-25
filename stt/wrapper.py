#!/usr/bin/env python3
import os
import sys
import struct
import tempfile
import logging

logging.basicConfig(stream=sys.stderr, level=logging.INFO,
                    format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)

from faster_whisper import WhisperModel

model_name   = os.environ.get("WHISPER_MODEL",        "tiny")
device       = os.environ.get("WHISPER_DEVICE",       "cpu")
compute_type = os.environ.get("WHISPER_COMPUTE_TYPE", "int8")
language     = os.environ.get("WHISPER_LANGUAGE")     or None

log.info("loading model=%s device=%s compute_type=%s", model_name, device, compute_type)
model = WhisperModel(model_name, device=device, compute_type=compute_type)
log.info("model ready")

stdin  = sys.stdin.buffer
stdout = sys.stdout.buffer

while True:
    header = stdin.read(4)
    if len(header) < 4:
        log.info("stdin closed, exiting")
        break

    length = struct.unpack(">I", header)[0]
    audio  = stdin.read(length)
    if len(audio) < length:
        log.warning("short read: expected %d got %d bytes", length, len(audio))
        break

    try:
        with tempfile.NamedTemporaryFile(suffix=".webm", delete=False) as f:
            f.write(audio)
            tmp = f.name
        try:
            segments, _ = model.transcribe(tmp, beam_size=5, language=language)
            transcript  = " ".join(s.text for s in segments).strip()
        finally:
            os.unlink(tmp)
    except Exception as exc:
        log.error("transcription error: %s", exc)
        transcript = ""

    stdout.write((transcript + "\n").encode())
    stdout.flush()
