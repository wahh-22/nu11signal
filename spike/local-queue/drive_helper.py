#!/usr/bin/env python3
"""Drives a separately built nu11signal helper over JSON Lines (spike only).

usage: drive_helper.py <helper-binary> <stderr-file> [play <startIndex> [seconds]]
Without "play" it only reads: authorize, playlists, the playlist tracks.
"""
import json, subprocess, sys, threading, time, queue

PLAYLIST = "Canciones favoritas"
binary, errpath = sys.argv[1], sys.argv[2]
err = open(errpath, "a")
p = subprocess.Popen([binary], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=err, text=True, bufsize=1)
lines = queue.Queue()
threading.Thread(target=lambda: [lines.put(l) for l in p.stdout], daemon=True).start()
events = []
n = 0

def call(cmd, timeout=20, **args):
    global n
    n += 1
    rid = str(n)
    p.stdin.write(json.dumps({"id": rid, "cmd": cmd, **args}) + "\n"); p.stdin.flush()
    end = time.time() + timeout
    while time.time() < end:
        try:
            msg = json.loads(lines.get(timeout=end - time.time()))
        except queue.Empty:
            break
        if msg.get("id") == rid:
            return msg
        events.append(msg)
    return {"timeout": cmd}

def drain(seconds):
    end = time.time() + seconds
    while time.time() < end:
        try:
            events.append(json.loads(lines.get(timeout=max(0.01, end - time.time()))))
        except queue.Empty:
            pass

print("authorize", call("authorize"))
pls = call("playlists")["result"]["playlists"]
pl = next(x for x in pls if x["name"] == PLAYLIST)
tracks = call("libraryPlaylist", playlistId=pl["id"])["result"]["tracks"]
print(json.dumps({"playlist": pl, "tracks": tracks}, ensure_ascii=False, indent=1))
if len(sys.argv) > 3 and sys.argv[3] == "play":
    start = int(sys.argv[4]); secs = float(sys.argv[5]) if len(sys.argv) > 5 else 3
    ids = [t["id"] for t in tracks if not t.get("libraryOnly")]
    t0 = time.time()
    print("playSongs", json.dumps(call("playSongs", ids=ids, startIndex=start), ensure_ascii=False))
    drain(secs)
    print("next", call("next")); drain(1.5)
    print("pause", call("pause")); print("stop", call("stop"))
    print("played_seconds", round(time.time() - t0, 1))
    for e in events:
        if "event" in e or "state" in e or "type" in e:
            print("event", json.dumps(e, ensure_ascii=False)[:400])
p.stdin.close(); p.wait(timeout=15)
