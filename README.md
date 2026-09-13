<div align="center">

# bililive-cli

Monitor bilibili live streams in your terminal

[Installation](#installation) • [Usage](#usage) • [Development](#development)

<!-- README-I18N:START -->

**English** | [中文](./README.zh.md)

<!-- README-I18N:END -->

</div>

bililive-cli (`bililive`) follows a bilibili live room and logs its audience interactions in CLI:

- danmaku (comments)
- gifts
- subscription purchases
- Super Chats
- enters
- likes
- follower events
- other room status changes
- send /reply to chats

<img src="img/demo.png" align="center" alt="demo"/>

## Installation

Install directly with Go 1.27+ (the binary will be named `bililive-cli` rather than `bililive`):

```bash
go install github.com/Arcadi4/bililive-cli@latest
```

Or download a prebuilt binary from the [GitHub releases](https://github.com/Arcadi4/bililive-cli/releases) page.

Or use the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/Arcadi4/bililive-cli/HEAD/install.sh | bash
```

## Usage

### Log in

```bash
bililive login
```

This prints a QR code in the terminal. Scan it with the bilibili app and confirm on your phone.

> [!NOTE]
> If QR login is unavailable, import a browser Cookie header instead: `bililive login --cookie "SESSDATA=...; bili_jct=..."`. `SESSDATA` is required while `bili_jct` enables sending comments.

You can find the session files at `~/.cache/bililive-cli/` on Linux or `~/Library/Caches/bililive-cli/` on macOS.

`bililive whoami` verifies the stored session against bilibili. `bililive logout` removes it.

### Watch a room

```bash
# room id, short id, or URL
bililive watch 42062
bililive watch 6
bililive watch https://live.bilibili.com/721

# your own room (requires login)
bililive watch

# plain output when piped
bililive watch 1878516995 | tee room.log
```

While watching, type into the bottom bar and press Enter to send a comment.

| Flag | Effect |
| --- | --- |
| `-q`, `--quiet` | Hide enter/like noise, keep gifts/comments/system |
| `--all` | Show raw protocol messages and metadata lines |
| `--plain` | Disable the bottom bar and key input (pipe mode) |
| `--debug` | Log protocol diagnostics to stderr |
| `--unicode` | Use plain unicode symbols instead of Nerd Font glyphs |

### Send a comment directly

```bash
bililive send 1878516995 "你好"
```

### Commands

| Command | Description |
| --- | --- |
| `bililive watch [room]` | Follow a live room and log its interactions |
| `bililive login` | Log in with a QR code (`--cookie` as fallback) |
| `bililive logout` | Remove the stored session |
| `bililive whoami` | Verify and show the logged-in account |
| `bililive send <room> <message>` | Send a comment without the TUI |

## Development

Requires Go 1.27.

```bash
task                                  # gofmt check, go vet, build
task run -- watch 1878516995 --plain  # run without building
task build                            # generate bin/bililive
```
