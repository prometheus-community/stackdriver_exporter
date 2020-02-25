include Makefile.common

DOCKER_IMAGE_NAME       ?= stackdriver-exporter

crossbuild: promu
	@echo ">> building cross-platform binaries"
	@$(PROMU) crossbuild
