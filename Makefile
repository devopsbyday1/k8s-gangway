# Copyright © 2017 Heptio
# Copyright © 2017 Craig Tracey
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PROJECT := gangway
# Where to push the docker image.
REGISTRY ?= ghcr.io/devopsbyday1
IMAGE := $(REGISTRY)/k8s-gangway
SRCDIRS := ./cmd/gangway
PKGS := $(shell go list ./cmd/... ./internal/...)

VERSION ?= latest

all: build

build:
	go build ./...

install:
	go install -v ./cmd/gangway/...

deps:
	go mod tidy && go mod verify

check: test vet

vet:
	go vet ./...

test:
	go test -v ./...

image:
	docker build . -t $(IMAGE):$(VERSION)

push:
	docker push $(IMAGE):$(VERSION)

.PHONY: all deps test image build install check vet
