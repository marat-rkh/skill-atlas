# Overview

Skill Atlas is a cli tool that analyzes git repositories and generates a map of available agent skills.

# Input

Any GitHub repository URL.

# Output

A map of available agent skills.

# Implementation details

Discovered skills can be grouped thematically, but this is not required.

# Technologies

- Go, standard library only.
- The `git` command-line tool, used at runtime to fetch repositories.