#!/usr/bin/env bash
version=local-test
time=$(date)
echo "$version"
go build -o .build/registryViewer \
         -ldflags="-s -w -X 'github.com/JLevconoks/registryViewer/cmd.buildTime=$time' -X 'github.com/JLevconoks/registryViewer/cmd.buildVersion=$version'" .