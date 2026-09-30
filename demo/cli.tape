# Terminal part of the skill-atlas demo. Run through record.sh, which puts skill-atlas on PATH.
Output out/cli.mp4

Set Shell "bash"
Set FontSize 15
Set Width 1920
Set Height 1080
Set Padding 24
Set TypingSpeed 35ms
Set Theme "Catppuccin Mocha"

Sleep 500ms
Type "# skill-atlas maps the agent skills of any GitHub repository"
Enter
Sleep 800ms
Type "skill-atlas"
Sleep 400ms
Enter
Wait
Sleep 4s

Ctrl+L
Type "# 1. The map: all skills, sorted by name"
Enter
Sleep 600ms
Type "skill-atlas https://github.com/vercel-labs/agent-skills"
Sleep 400ms
Enter
Wait@60s
Sleep 7s

Ctrl+L
Type "# 2. --group groups skills whose names start with the same word"
Enter
Sleep 600ms
Type "skill-atlas --group https://github.com/vercel-labs/agent-skills"
Sleep 400ms
Enter
Wait@60s
Sleep 7s

Ctrl+L
Type "# 3. The web interface shows the same map in a browser"
Enter
Sleep 600ms
Type "skill-atlas serve"
Sleep 400ms
Enter
Sleep 3s
Hide
Ctrl+C
