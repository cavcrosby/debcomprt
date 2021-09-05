# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
TARGET_EXEC = debcomprt
SHELL = /usr/bin/sh
TEST_BIN_DIR = /usr/local/bin

# executables
GO = go
ADDLICENSE = addlicense
executables = \
	${GO}

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = $(shell if [ -n "${GOBIN}" ]; then echo "${GOBIN}"; else echo "${GOPATH}/bin"; fi)
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}

# targets
HELP = help
SETUP = setup
INSTALL = install
TEST = test
CLEAN = clean

# to be passed in at make runtime
COPYRIGHT_HOLDERS =

# simply expanded variables
src := $(shell find . \( -type f \) -and \( -iname '*.go' \) -and \( -not -iregex './vendor.*' \))

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Available make targets:'
>	@echo '  ${TARGET_EXEC}          - the decomprt binary'
>	@echo '  ${INSTALL}            - installs the local decomprt binary (pathing: ${prefix})'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADDLICENSE}         - adds license header to src files'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Public make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holders/authors'
>	@echo '                          (e.g. John Smith, Alice Smith)'

${TARGET_EXEC}: debcomprt.go
>	${GO} build -o "${TARGET_EXEC}" -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${GO} install

.PHONY: ${TEST}
${TEST}:
>	sudo PATH="${PATH}" ${GO} test -v -mod vendor

# for reference: https://github.com/google/addlicense
.PHONY: ${ADDLICENSE}
${ADDLICENSE}:
	# DISCUSS(cavcrosby): this is only a tool used when doing development on this
	# go module. That, and the tool is written in golang. Probably do not want to
	# vendorize the code for the tool, but perhaps we could specify a different file
	# for dependencies that are tools.
>	@[ -z $$(command -v "${ADDLICENSE}") ] || { echo "No ${ADDLICENSE} in PATH"; exit 1; }
>	@[ -n "${COPYRIGHT_HOLDERS}" ] || { echo "COPYRIGHT_HOLDERS was not passed into make"; exit 1; }
>	addlicense -l apache -c "${COPYRIGHT_HOLDERS}" ${src}

.PHONY: ${CLEAN}
${CLEAN}:
>	rm --force "${TARGET_EXEC}"
