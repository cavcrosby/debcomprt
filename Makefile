# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
TARGET_EXEC = debcomprt
SHELL = /usr/bin/sh
TEST_BIN_DIR = /usr/local/bin

# executables
GOC = go
executables = \
	${GOC}

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
>	@echo '  ${CLEAN}              - remove files created by other targets'

${TARGET_EXEC}: debcomprt.go
>	${GOC} build -o "${TARGET_EXEC}" -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${GOC} install

.PHONY: ${TEST}
${TEST}:
>	sudo PATH="${PATH}" ${GOC} test -v -mod vendor

.PHONY: ${CLEAN}
${CLEAN}:
>	rm --force "${TARGET_EXEC}"
