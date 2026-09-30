---
description: File and fix a bug
argument-hint: number of test cases | description
---

## Context
Parse $ARGUMENTS to get the following values

- [number of test cases]: Minimum number of test cases to author (0 means none)
- [description]: Description of the bug

## Tasks
1. Discombobulate the bug [description]
2. Discover the source and causeof the bug
3. Determine the fix and the test case(s) to reproduce the bug
4. consult with the gatekeeper(s) regarding paradigm 1
5. make a plan
6. execute the plan
7. ensure 'make test' will include the new test cases