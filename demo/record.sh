#!/usr/bin/env bash
# Records the demo video: the CLI with VHS (cli.tape), the web interface with Playwright (web.mjs), joined with ffmpeg.
#
# Usage: demo/record.sh [output.mp4]    (default: demo/out/skill-atlas-demo.mp4)
# Needs: go, vhs and ffmpeg (`brew install vhs` brings both), node, and Google Chrome (or another Chrome at CHROME_PATH).
# Port 8080 must be free. The demo lists the JetBrains repositories twice with the GitHub API, which allows 60 requests
# per hour without authentication; set GITHUB_TOKEN if that is not enough.
set -euo pipefail

demo=$(cd "$(dirname "$0")" && pwd)
out="$demo/out"
video=${1:-$out/skill-atlas-demo.mp4}
case $video in /*) ;; *) video="$PWD/$video" ;; esac

for tool in go vhs ffmpeg node npm curl; do
  command -v "$tool" >/dev/null || { echo "record.sh: $tool is not installed" >&2; exit 1; }
done
if lsof -nP -iTCP:8080 -sTCP:LISTEN >/dev/null; then
  echo "record.sh: port 8080 is in use; stop the running skill-atlas serve first" >&2
  exit 1
fi

rm -rf "$out/web-video"
mkdir -p "$out/bin"
go build -C "$demo/.." -o "$out/bin/skill-atlas" .
export PATH="$out/bin:$PATH"

cd "$demo"
[ -d node_modules ] || npm ci
npx playwright install ffmpeg

echo "Recording the CLI..."
vhs cli.tape

echo "Recording the web interface..."
skill-atlas serve > "$out/serve.log" 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null || true' EXIT
for _ in $(seq 50); do
  curl -sf http://127.0.0.1:8080/ >/dev/null && break
  sleep 0.2
done
web=$(node web.mjs)

echo "Joining the parts..."
ffmpeg -loglevel error -y -i out/cli.mp4 -i "$web" -filter_complex \
  "[0:v]fps=30,scale=1920:1080,setsar=1[cli];[1:v]fps=30,scale=1920:1080,setsar=1[web];[cli][web]concat=n=2:v=1:a=0[v]" \
  -map "[v]" -c:v libx264 -preset slow -crf 18 -pix_fmt yuv420p -movflags +faststart "$video"
echo "Recorded $video"
