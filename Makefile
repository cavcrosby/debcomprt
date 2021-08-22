# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
TARGET_EXEC = debcomprt
SHELL = /usr/bin/sh

# executables
GOC = go
executables = \
	${GOC}

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = $(shell ${GOC} list -f '{{.Target}}')
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}

# targets
HELP = help
SETUP = setup
INSTALL = install
CLEAN = clean

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Available make targets:'
>	@echo '  ${TARGET_EXEC}          - the decomprt binary'
>	@echo '  ${SETUP}              - installs the go dependencies for this project'
>	@echo '  ${INSTALL}            - installs the local decomprt binary (pathing: ${prefix})'
>	@echo '  ${CLEAN}              - remove files created by other targets'

${TARGET_EXEC}: debcomprt.go
>	${GOC} build -o "${TARGET_EXEC}"

.PHONY: ${SETUP}
${SETUP}: 
>	${GOC} mod download

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${GOC} install

.PHONY: ${CLEAN}
${CLEAN}:
>	rm --force "${TARGET_EXEC}"
