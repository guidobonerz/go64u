## go64u

Ultimate64 CLI

Current functions
* show rest api version
* show device information (fpga version, etc.)
* mount/unmount disk images
* reset/reboot/pause/resume machine
* poke/peek value in/from memory
* write message on screen(base:0x400) at x,y
* load and run local prg/crt over dma
* make screenshot as png
* show screenshot directly in console (sixel, if supported)
* show current vic/bank setup (d011/d016/d018/dd00)
* navigate through internal storage via ls/cd over ftp connection
* terminal mode
* selectable audiostream player

Some commands are only available in terminal mode
* cd (change directory)
* asc (audio stream controller)
* stream (stream to your favourite platform, usually needs a streaming key)

### Folder structure with icons
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/list.png)
### Audio Stream Player - Terminal
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/streamplayer.png)
### Audio Stream Player - GUI
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/gui_streamplayer.png)

## Todo's
* add gui frontend (currently available experimental, still buggy)
* twitch stream
* add streaming client for video (monitor mode)
* record audio
* list and change dir (local and remote)
* disassembler with dialect option

## Installation
Create an environment variable **GO64U_CONFIG_PATH** where the **.go64u.yaml** file is located

The structure of the file is currently as follows

```
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
ScreenshotFolder: <path to screenshot folder>
```
NOTE: If you have more than one Ultimate64 board, you have to choose different ports for each board

NOTE: If you want to stream the u64 stream via rtmp to twitch, youtube, etc. you need to install ffmpeg

## How to start

### Option1 - Terminal Mode

go64u <command>

### Option2 - Terminal Mode(REPL)

go64u --terminal

### Option3 - GUI Mode

go64u --gui

## How to stream to twitch or other platforms
start go64u in terminal mode : .\go64u.exe --terminal
then type **stream platform_name**

