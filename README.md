# go64u

Ultimate64 Remote CLI

**go64u** is a tool for remote interaction with the Ultimate64 computer.

```
go64u [command] [flags]
```

## How to start

### Option 1 - UI Mode

```
go64u
```

### Option 2 - Terminal Mode (REPL)

```
go64u --terminal
```

### Option 3 - CLI Mode

```
go64u <command>
```

---

## Global Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--terminal` | – | bool | `false` | Run the application in terminal (REPL) mode |
| `--device` | `-d` | string | *(from config)* | Set device. Needed in non-terminal mode |

---

## Machine Commands

### `devices`
Show device configurations.

### `message [message] [x] [y]`
Writes a message on screen at a given position.

### `pause`
Pauses the U64 by pulling the DMA line low at a safe moment. This stops the CPU. Note that this does not stop any timers.

### `poweroff`
Shuts down the U64. Note that it is likely that you won't receive a valid response.

### `readmem [address]`
Reads several bytes from memory by a given length.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--length` | `-l` | uint16 | `1` | Length of data to read |
| `--output` | `-o` | string | `output.bin` | Output file name |
| `--type` | `-t` | string | `file` | Output type: `file` or `bin` |

### `reboot`
Reboots the U64. Re-initializes the cartridge configuration and sends a reset to the machine.

### `reset`
Resets the U64. The current configuration is not changed.

### `resume`
Resumes the U64 after pause. The DMA line is released and the CPU will continue where it left off.

### `togglemenu`
Toggles the on-screen menu. Does the same thing as pressing the Menu button on an 1541 Ultimate cartridge or briefly pressing the Multi Button on the Ultimate 64.

### `writemem [address] [value]`
Sets one byte in memory (POKE). Writes a byte value (00-ff) to a memory address (0-ffff).

---

## Runner Commands

### `crt [file]`
Loads a cartridge file into the U64 and automatically starts it. The machine resets with the attached cartridge active. It does not alter the configuration of the Ultimate.

### `load [file] [address]`
Loads a program into the U64. The machine resets and loads the attached program into memory using DMA. It does not automatically run the program.

### `mount [drive] [file]`
Mounts a disk image (d64/g64/d71/g71/d81) on a given drive.

### `run [file] [address]`
Loads a program into the U64 and automatically starts it. The machine resets, loads the attached program into memory using DMA, then automatically runs the program.

### `unmount [drive]`
Unmounts a disk image from a given drive.

---

## File Commands

### `ls [path/diskimage]`
List files of the internal drive (USB Stick, SD Card, Disk Images, etc.).

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--memaddress` | `-m` | bool | `false` | Display the start address of a program if possible |
| `--filter` | `-f` | string | *(empty)* | Filter the list by a match pattern like `*.prg` |

---

## Drive Commands

### `drives`
Show drive info.

---

## Stream Commands

### `audio [command]`
Starts/Stops the audio stream.

| Command | Description |
|---------|-------------|
| `audio start` | Start the audio stream |
| `audio stop` | Stop the audio stream |

### `debug [command]`
Starts/Stops the debug stream. Audio and video streams will be stopped.

| Command | Description |
|---------|-------------|
| `debug start` | Start the debug stream (audio/video streams will be stopped) |
| `debug stop` | Stop the debug stream |

### `video [command]`
Starts/Stops the video stream.

| Command | Description |
|---------|-------------|
| `video start` | Start the video stream |
| `video stop` | Stop the video stream |

---

## VIC Commands

### `screenmem`
Shows the current VIC states of D011/D016/D018 and the memory bank setup.

### `screenshot`
Takes a screenshot of the current screen.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--scale` | int | `100` | Scale factor in percent (%) |

---

## Platform Commands

### `info`
Show device info.

### `online`
Check if the selected device is online (responds to HTTP requests).

---

## Terminal Mode (REPL)

Terminal mode is started with `go64u --terminal`. It provides an interactive REPL where all standard commands are available plus the following **terminal-only commands**:

### `asc`
Interactive audio stream controller. Lets you select from configured devices, play/stop audio streams, and switch between devices interactively.

### `cd [path/diskimage]`
Change the folder on the internal drive. Only available in terminal mode because it maintains a persistent working directory across commands.

### `device [device_key]`
Switch the active device within the terminal session. Shows the current device if no key is provided.

### `query`
Query packages matching a filter. By default the filter is set to the current year and type=d64.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | *(empty)* | Filter by name |
| `--group` | string | *(empty)* | Filter by group |
| `--handle` | string | *(empty)* | Filter by handle |
| `--category` | string | *(empty)* | Filter by category |
| `--repo` | string | *(empty)* | Filter by repository |
| `--subcat` | string | *(empty)* | Filter by subcategory |
| `--year` | string | *(current year)* | Filter by year |
| `--rating` | string | *(empty)* | Filter by rating |
| `--type` | string | *(empty)* | Filter by type |
| `--latest` | string | *(current month)* | Filter by latest |
| `--offset` | int | `0` | Result offset |
| `--limit` | int | `80` | Result limit |
| `--ignoreDefaults` | bool | `false` | Ignore default filters |
| `--get` | bool | `false` | Download the files |

### `quit`
Quit the terminal and exit the application.

### `stream`
Stream to your favourite streaming platform (e.g. Twitch/YouTube). Starts both video and audio streams and pipes them through an RTMP encoder.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--target` | string | *(empty)* | Streaming platform (e.g. twitch, youtube) |
| `--record` | string | *(empty)* | Record locally: `audio`, `video`, or `both` |
| `--no-overlay` | string | *(empty)* | Disable overlay for: `stream`, `record`, or `both` |

---

### Database Sub-REPL

After entering `query` (or its dedicated database mode), the following sub-commands are available within the database REPL:

#### `category`
Show categories.

#### `filter`
Show/set active filters.

#### `list`
List filtered results.

#### `quit`
Leave the database sub-REPL.

---

## GUI Frontend (Gio UI)

The graphical interface is launched without the `--terminal` flag and uses the **Gio UI** framework (`gioui.org`). The window has a resolution of 800x680 pixels with a dark Material Design theme.

### Layout

The layout consists of three areas:

1. **Toolbar** (top) – Control buttons for the selected device
2. **Main area** – Device cards (left) + Video monitor grid (right)
3. **Footer** (bottom) – Status line with hover hints

### Toolbar Functions

| Button | Function |
|--------|----------|
| Play/Stop | Start/Stop all streams for the selected device |
| Pause | Pause the U64 (DMA line low) |
| Audio | Enable/Disable audio monitoring with playback |
| Video | Enable/Disable video monitoring |
| Snapshot | Save a screenshot as PNG to the configured folder |
| Record | Start/Stop MP4 recording (video + audio) |
| Cast | Start/Stop streaming to a platform (e.g. Twitch) |
| Overlay | Enable/Disable the stream overlay |
| CRT | Enable/Disable the CRT scanline effect |
| Reset | Send a reset to the U64 |
| Power Off | Shut down the U64 |

### Device Cards

Each configured device is displayed as a card with rounded corners. The card shows:

- Device name and description
- Online status indicator (green = online, red = offline, gray = unchecked)

The online check runs automatically every 5 seconds in the background.

### Video Monitor

- Displays active video feeds in a grid layout
- Supports multiple simultaneous device streams
- Native resolution: 384x272 pixels (Ultimate64)
- Scaling with rounded corners
- Optional CRT scanline effect (sine wave-based brightness modulation)

### Audio Waveform

- Real-time stereo waveform visualization
- Two channels with visible gap
- Updated live during audio playback

### Drag & Drop (Windows)

On Windows, files can be dragged and dropped onto a device monitor window. The file is automatically routed to the correct device based on the drop position in the grid.

### Recording & Streaming

- **Recording**: Saves as MP4 to the configured `RecordingFolder`
- **Casting**: Streams live to configured platforms (e.g. Twitch) via the `StreamingTargets` configuration
- Both use the `StreamRenderer` for video encoding, audio mixing, and optional overlay compositing

### Audio System

- Framework: `ebitengine/oto/v3`
- 48 kHz sample rate, stereo, 16-bit signed LE
- Real-time playback and monitoring

---

## Screenshots

### Folder structure with icons
![Styled Directory](https://github.com/guidobonerz/go64u/blob/main/doc/list.png)

### Audio Stream Player - Terminal
![Audio Stream Player](https://github.com/guidobonerz/go64u/blob/main/doc/streamplayer.png)

### Stream Player - Monitor
![Stream Player Monitor](https://github.com/guidobonerz/go64u/blob/main/doc/gui_streamplayer.png)

---

## Installation

Create an environment variable **GO64U_CONFIG_PATH** where the **.go64u.yaml** file is located.

The structure of the file is currently as follows:

```yaml
LogLevel: quiet
Devices:
  DEVICE_NAME[n]:
    Description: "Device name"
    IsDefault: true
    IpAddress: <ip of device>
    VideoPort: 11000
    AudioPort: 11001
    DebugPort: 11002
StreamingTargets:
  <name of the streaming platform, e.g. twitch>: rtmp://live.twitch.tv/app/<stream_key>
  <second platform...>: rtmp://
LogLevel: ffmpeg_loglevel
DumpFolder: <path to dump folder>
RecordingFolder: <path to recording folder>
ScreenshotFolder: <path to screenshot folder>
DownloadFolder: <path to download folder>
Overlay:
    X: 700
    Y: 200
    WIDTH: 800
    HEIGHT: 600
    ImagePath: <path to overlay image>
```

> **Note:** If you have more than one Ultimate64 board, you have to choose different ports for each board.

> **Note:** If you want to stream the U64 stream via RTMP to Twitch, YouTube, etc. you need to install ffmpeg.

---

## Todo

- List and change dir (local and remote)
- Disassembler with dialect option
