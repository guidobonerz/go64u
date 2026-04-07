#!/bin/bash
module_name=$(awk '/^module /{split($2,a,"/"); print a[length(a)]}' go.mod)
go build -trimpath -ldflags "-w -s" -o "$module_name" main.go

if [ "$1" = "-p" ]; then
    upx --best "$module_name"
fi
