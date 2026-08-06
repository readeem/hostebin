# Agent skills

Drop-in instructions that teach a coding agent how to publish its output with
hostebin. Each subdirectory is one skill: a `SKILL.md` with YAML frontmatter
(`name`, `description`) plus optional reference files the agent loads on demand.

| Skill | Use for |
| --- | --- |
| [`hostebin/`](hostebin/) | Publishing pages, reports, and assets to a shareable URL; listing and deleting bundles |
| [`beautiful-html-reports/`](beautiful-html-reports/) | Designing the page before you publish it — templates, tokens, icons, and components for readable HTML |

The two are meant to be used together: `beautiful-html-reports` produces the file,
`hostebin` puts it online.

## Install

**Claude Code** — copy the skills into a skills directory:

```sh
# just this project
mkdir -p .claude/skills && cp -r skills/hostebin skills/beautiful-html-reports .claude/skills/

# every project on this machine
mkdir -p ~/.claude/skills && cp -r skills/hostebin skills/beautiful-html-reports ~/.claude/skills/
```

Symlink instead of copying (`ln -s "$PWD/skills/hostebin" ~/.claude/skills/hostebin`)
to track updates.

**Other agents** (Codex, Cursor, Copilot, custom harnesses) — `SKILL.md` is plain
Markdown. Point the agent at it, or append it to `AGENTS.md`, `CONVENTIONS.md`, or
the system prompt:

```sh
cat skills/hostebin/SKILL.md >> AGENTS.md
```

Keep the skill directory reachable either way: `beautiful-html-reports/SKILL.md`
refers to the template and reference files sitting next to it.

## Environment

A skill only helps if the CLI can reach a server. Give the agent's shell:

```sh
export HOSTEBIN_SERVER=https://hostebin.example.com
export HOSTEBIN_TOKEN=...
```

Or write them once to `~/.config/hostebin/config.json` — see the
[README](../README.md#configuration).
