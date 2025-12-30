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

### Folder structure with icons
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/list.png)
### Audio Stream Player - Terminal
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/streamplayer.png)
### Audio Stream Player - GUI
![Styled Directory](https://github.com/guidobonerz/go64u/blob/develop/doc/gui_streamplayer.png)

## Todo's
* add gui frontend (currently available experimental, still buggy)
* add streaming client for video
* record audio
* list and change dir (local and remote)
* disassembler with dialect option

## Installation
Create an environment variable **GO64U_CONFIG** 
which points to a configuration **yaml** file

The structure of the file is currently as follows

```
Devices:
  DEVICE_NAME1:
    Description: "Device name"
    IsDefault: true
    IpAddress: <ip of device>
    VideoPort: 11000
    AudioPort: 11001
    DebugPort: 11002
  DEVICE_NAME2:
    Description: "Device name"
    IpAddress: <ip of device>
    VideoPort: 21000
    AudioPort: 21001
    DebugPort: 21002
DumpFolder: <path to dump folder>
ScreenshotFolder: <path to screenshot folder>
```

## How to start

### Option1 - Terminal Mode

go64u <command>

### Option2 - Terminal Mode(REPL)

go64u --terminal

### Option3 - GUI Mode

go64u --gui


