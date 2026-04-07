param(
    [switch]$p
)

$moduleName = (Select-String -Path "go.mod" -Pattern "^module\s+(.+)$").Matches.Groups[1].Value.Split("/")[-1]
go build -trimpath -ldflags "-w -s" -o "$moduleName.exe" main.go

if ($p) {
    upx --best "$moduleName.exe"
}
