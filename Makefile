# tool variables
GO=GO111MODULE=on go
DEP=dep
GOMETALINTER=gometalinter

# verbosity
Q=@


## make rules

build:
	$(Q)$(GO) build $(GO_BUILD_FLAGS)

test:
	$(Q)$(GO) test

lint:
	$(Q)$(GOMETALINTER) $(GOMETALINTER_FLAGS) .

.PHONY: all build get install test lint

