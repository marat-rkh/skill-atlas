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
Type "# skill-atlas maps the agent skills of any GitHub repository or organization"
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
Type "# 3. An organization URL maps all of the organization's repositories, except forks"
Enter
Sleep 600ms
Type "time skill-atlas https://github.com/JetBrains > out/jetbrains.txt"
Sleep 400ms
Enter
Sleep 3s
# Analyzing hundreds of repositories takes a while, so the wait is cut; `time` shows how long it took.
Hide
Wait@5m
Show
Sleep 3s
Type "head -n 32 out/jetbrains.txt"
Sleep 400ms
Enter
Sleep 7s

Ctrl+L
Type "# The map of an organization has a section for each repository with skills"
Enter
Sleep 600ms
Type "grep '^JetBrains' out/jetbrains.txt"
Sleep 400ms
Enter
Sleep 7s

Ctrl+L
Type "# 4. The web interface shows the same maps in a browser"
Enter
Sleep 600ms
Type "skill-atlas serve"
Sleep 400ms
Enter
Sleep 3s
Hide
Ctrl+C
