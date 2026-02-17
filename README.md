# Prompter

A local web app that helps open source contributors create high-quality, AI-assisted prompt requests for project maintainers.

Instead of writing a vague feature request, Prompter guides a conversation with Claude that explores the target repository's codebase, asks clarifying questions, and generates a well-structured, actionable prompt — published as a GitHub issue that maintainers can feed directly into their AI coding agent.

## Prerequisites

- [Go](https://go.dev/) 1.25.5+
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) (`claude`)
- [GitHub CLI](https://cli.github.com) (`gh`), authenticated via `gh auth login`
- `git`

## Install

```bash
go install github.com/esnunes/prompter/cmd/prompter@latest
```

Or build from source:

```bash
go build ./cmd/prompter
```

## Usage

```bash
prompter github.com/owner/repo
```

Prompter will clone the target repo (or pull if already cloned), start a local web server, and open your browser. From the UI you can:

1. Create a new prompt request
2. Have a guided conversation with Claude, which explores the repo and asks clarifying questions
3. Review the generated prompt
4. Publish it as a GitHub issue

Data is stored locally in `~/.cache/prompter/`.

## License

MIT
