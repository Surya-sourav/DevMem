# devmem

Developers have a context window too. DevMem is the fix.

A CLI tool that crawls your codebase, documents every module using AI, and
keeps that documentation current on every git commit. Every change is captured,
classified, and written to a changelog entry tied permanently to its commit hash.

Documentation that writes itself. Memory that does not fade.

```bash
$ devmem auth login
API key saved to keychain.

$ devmem init

  Crawling codebase...           done  (312 files, 0.1s)
  Detecting modules...           found 7 candidates

  Detected modules:
    auth       src/auth/         (score: 7)
    api        src/api/          (score: 6)
    db         internal/db/      (score: 5)
    worker     internal/worker/  (score: 4)
    config     pkg/config/       (score: 3)

  Confirm? [Y/n]: Y

  Analysing modules (7 total, max 5 parallel)...
    [1/7] auth      ✓ done  (1.2s)
    [3/7] api       ✓ done  (1.4s)
    [2/7] db        ✓ done  (1.8s)
    [5/7] config    ✓ done  (0.9s)
    [4/7] worker    ✓ done  (2.1s)

  Generating master architecture...  ✓ done  (2.3s)

  devmem initialised. 7 modules documented.

$ devmem query "how does authentication work?"
auth issues and validates JWTs, exposes middleware, and depends on db + config.
```

## The problem

Codebases grow faster than any one developer can track, and AI-assisted development increases that rate again. AI-assisted development produces code faster than documentation can follow.

Documentation written once starts drifting as soon as the next refactor lands. The only documentation that stays honest is documentation written automatically at the moment of change by reading the diff.

## How it works

1. DevMem crawls the repository tree and applies ignore rules during traversal.
2. DevMem scores directories to detect module boundaries.
3. DevMem asks you to confirm the module set before analysis starts.
4. DevMem analyzes confirmed modules in parallel and writes per-module docs.
5. DevMem generates a master architecture document and dependency graph.

On every capture run, DevMem reads the latest diff, classifies changes, patches only affected docs, and writes a commit-linked changelog entry.

## Install

#### Homebrew

```bash
brew tap surya-sourav/tap
brew install devmem
```

#### Go install

```bash
go install github.com/surya-sourav/devmem@latest
```

#### Linux

```bash
curl -sL https://github.com/surya-sourav/devmem/releases/latest/download/devmem_linux_amd64.tar.gz \
  | tar xz && sudo mv devmem /usr/local/bin/
```

Windows users: download the latest `.zip` from [Releases](https://github.com/surya-sourav/devmem/releases) and add to PATH.

## Quick start

```bash
devmem auth login
cd your-project
devmem init
devmem query "explain the overall architecture"
```

## Commands

| Command | Description | When to run |
|---|---|---|
| `devmem auth login` | Save your Anthropic API key to the system keychain | Once, on first install |
| `devmem init` | Crawl the repo, detect modules, generate all docs | Once per project, re-run to refresh |
| `devmem capture` | Diff the latest commit, update affected module docs, write changelog | After every commit (or via git hook) |
| `devmem status` | Show which modules have undocumented changes | In CI or before a PR |
| `devmem query "<question>"` | Ask a natural language question about your codebase | Anytime |

## What gets generated

```text
.devmem/
├── config.json                  # module map, model config, ignore rules
├── state.json                   # last commit hash, init timestamp
├── tree.json                    # full filesystem snapshot
├── docs/
│   ├── master-architecture.md   # system overview, module table, data flow
│   ├── master-architecture.mermaid  # module dependency graph
│   └── modules/
│       ├── auth.md              # per-module: purpose, API, deps, changelog
│       └── ...
└── changelog/
    └── abc123f.md               # one entry per captured commit
```

The entire `.devmem/` directory is designed to be git-tracked - it diffs cleanly, reviews well in pull requests, and accumulates a meaningful history of your codebase's evolution.

## Configuration

For the full reference, use the docs page.

```json
{
  "ai_model": "claude-sonnet-4-20250514",
  "max_concurrent": 5,
  "modules": {
    "auth": { "paths": ["src/auth"] }
  }
}
```

## Git hook

DevMem can install a post-commit hook so capture runs after each commit.

```bash
# Offered automatically during devmem init
# Or install manually:
devmem capture --install-hook
```

Once installed, `devmem capture` runs after every commit automatically - no manual intervention required.

## Requirements

- Go 1.21 or higher (for `go install` only - pre-built binaries have no Go dependency)
- Git
- Anthropic API key - get one at [console.anthropic.com](https://console.anthropic.com)

## Docs

Full documentation - commands, internals, file schemas, and configuration reference - at [surya-sourav.github.io/devmem](https://surya-sourav.github.io/devmem) or in [`docs.html`](./docs.html).

## License

```
MIT
```
