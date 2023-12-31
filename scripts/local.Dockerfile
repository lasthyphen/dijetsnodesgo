# syntax=docker/dockerfile:experimental

# This Dockerfile is meant to be used with the build_local_dep_image.sh script
# in order to build an image using the local version of coreth

# Changes to the minimum golang version must also be replicated in
# scripts/build_dijets.sh
# scripts/local.Dockerfile (here)
# Dockerfile
# README.md
# go.mod
FROM golang:1.18.5-buster

RUN mkdir -p /go/src/github.com/ava-labs

WORKDIR $GOPATH/src/github.com/ava-labs
COPY avalanchego avalanchego
COPY coreth coreth

WORKDIR $GOPATH/src/github.com/ava-labs/avalanchego
RUN ./scripts/build_dijets.sh
RUN ./scripts/build_coreth.sh -c ../coreth -e $PWD/build/plugins/evm

RUN ln -sv $GOPATH/src/github.com/ava-labs/avalanche-byzantine/ /avalanchego
