PREFIX ?= $(HOME)/.local
LABEL  := com.laduwka.gitgit-sync
PLIST  := $(HOME)/Library/LaunchAgents/$(LABEL).plist
DOMAIN := gui/$(shell id -u)

.PHONY: build test install-launchd uninstall-launchd

build:
	go build -o gitgit ./cmd/main.go

test:
	go test ./...

install-launchd: build
	install -d $(PREFIX)/bin $(HOME)/Library/Logs
	install -m 0755 gitgit $(PREFIX)/bin/gitgit
	install -m 0755 contrib/launchd/gitgit-sync $(PREFIX)/bin/gitgit-sync
	sed -e 's|@PREFIX@|$(PREFIX)|g' -e 's|@HOME@|$(HOME)|g' contrib/launchd/$(LABEL).plist.in > $(PLIST)
	plutil -lint $(PLIST)
	-launchctl bootout $(DOMAIN)/$(LABEL)
	launchctl bootstrap $(DOMAIN) $(PLIST)

uninstall-launchd:
	-launchctl bootout $(DOMAIN)/$(LABEL)
	rm -f $(PLIST) $(PREFIX)/bin/gitgit-sync
