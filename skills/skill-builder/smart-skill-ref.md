# Best Practices for Designing Claude Code Skills

Based on official Anthropic documentation and the Agent Skills specification, here are the key design principles:

---

## 1. Core Principles

| Principle | Description |
|-----------|-------------|
| **Be concise** | Context window is shared; every token competes with conversation history |
| **Assume Claude is smart** | Only add context Claude doesn't already have |
| **Progressive disclosure** | Load detailed info only when needed |
| **Test with real usage** | Iterate based on actual agent behavior, not assumptions |

---

## 2. Naming Conventions

Use **gerund form** (verb + -ing) for clarity:

```yaml
# Good (gerund form)
name: processing-pdfs
name: analyzing-spreadsheets
name: testing-code

# Acceptable alternatives
name: pdf-processing
name: process-pdfs

# Avoid
name: helper        # too vague
name: utils         # too generic
name: documents     # doesn't describe action
```

**Requirements**: lowercase, numbers, hyphens only; max 64 characters; no reserved words (anthropic, claude)

---

## 3. Write Effective Descriptions

The `description` field is **critical for skill discovery**—Claude uses it to select from potentially 100+ skills.

```yaml
# Good - specific with trigger keywords
description: Extract text and tables from PDF files, fill forms, merge documents.
  Use when working with PDF files or when the user mentions PDFs, forms, or document extraction.

# Bad - too vague
description: Helps with documents
description: Processes data
```

**Always write in third person** (not "I can help you" or "You can use this")

---

## 4. Structure for Progressive Disclosure

```
my-skill/
├── SKILL.md              # Main instructions (<500 lines)
├── REFERENCE.md          # Detailed docs (loaded on demand)
├── EXAMPLES.md           # Usage examples (loaded on demand)
└── scripts/
    └── validate.py       # Utility scripts (executed, not loaded)
```

**Token loading stages:**
1. **Startup**: Only `name` + `description` (~100 tokens)
2. **Activation**: Full SKILL.md (<5000 tokens recommended)
3. **On-demand**: Reference files only when needed

---

## 5. Set Appropriate Degrees of Freedom

| Freedom Level | When to Use | Example |
|---------------|-------------|---------|
| **High** | Multiple approaches valid | Code review guidelines |
| **Medium** | Preferred pattern exists | Template with parameters |
| **Low** | Operations are fragile | Exact migration script |

Think of Claude navigating a path:
- **Narrow bridge**: Provide exact guardrails (database migrations)
- **Open field**: Give general direction (code reviews)

---

## 6. Use Workflows for Complex Tasks

Provide checklists Claude can track:

````markdown
## PDF Form Filling Workflow

Copy this checklist:
```
- [ ] Step 1: Analyze form (run analyze_form.py)
- [ ] Step 2: Create field mapping
- [ ] Step 3: Validate mapping
- [ ] Step 4: Fill the form
- [ ] Step 5: Verify output
```
````

---

## 7. Implement Feedback Loops

The **"run validator → fix errors → repeat"** pattern greatly improves quality:

```markdown
1. Make edits to document.xml
2. **Validate immediately**: python validate.py
3. If validation fails:
   - Review error message
   - Fix issues
   - Validate again
4. Only proceed when validation passes
```

---

## 8. Avoid Common Anti-Patterns

| Anti-Pattern | Better Approach |
|--------------|-----------------|
| Deeply nested file references | Keep references one level deep from SKILL.md |
| Too many options | Provide a sensible default with escape hatch |
| Time-sensitive info | Use "old patterns" collapsible section |
| Inconsistent terminology | Pick one term and use it throughout |
| Windows-style paths (`\`) | Always use forward slashes (`/`) |
| Voodoo constants | Document why each value was chosen |

---

## 9. Template and Examples Patterns

**For strict requirements:**
````markdown
ALWAYS use this exact template:
```markdown
# [Title]
## Executive summary
## Key findings
## Recommendations
```
````

**For flexible guidance:**
````markdown
Sensible default format—adapt as needed:
```markdown
# [Title]
[Adjust sections based on what you discover]
```
````

**Show input/output examples:**
````markdown
**Example:**
Input: Added user authentication with JWT
Output:
```
feat(auth): implement JWT-based authentication
```
````

---

## 10. Evaluation-Driven Development

Build evaluations **before** writing extensive documentation:

1. **Identify gaps**: Run Claude on tasks without a skill; document failures
2. **Create evaluations**: Build 3+ scenarios testing those gaps
3. **Establish baseline**: Measure performance without the skill
4. **Write minimal instructions**: Just enough to pass evaluations
5. **Iterate**: Test, compare, refine

---

## 11. Develop Iteratively with Claude

Use the **Claude A / Claude B pattern**:

1. **Claude A** (expert): Helps you design and refine the skill
2. **Claude B** (user): Tests the skill on real tasks
3. Observe Claude B's behavior → bring insights back to Claude A → iterate

---

## 12. Context Engineering Principles

From Anthropic's engineering guidance:

- **Find the minimal effective set**: Smallest possible high-signal tokens
- **Use just-in-time loading**: Maintain lightweight identifiers, load data at runtime
- **Enable exploration**: Let Claude incrementally discover relevant context
- **Implement compaction**: For long tasks, summarize history before hitting limits

---

## Quick Checklist

- [ ] Description is specific with trigger keywords
- [ ] SKILL.md body under 500 lines
- [ ] File references one level deep
- [ ] Consistent terminology throughout
- [ ] No time-sensitive information
- [ ] Workflows have clear steps
- [ ] Feedback loops for quality-critical tasks
- [ ] Tested with Haiku, Sonnet, and Opus
- [ ] At least 3 real-world test scenarios

---

## Sources

- [Skill authoring best practices - Claude Docs](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
- [Agent Skills Specification](https://agentskills.io/specification)
- [Effective context engineering for AI agents - Anthropic](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills)
- [awesome-claude-skills repositories](https://github.com/ComposioHQ/awesome-claude-skills)
