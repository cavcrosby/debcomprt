FROM golang:1.17-bullseye

ARG BRANCH
ARG COMMIT
LABEL tech.cavcrosby.debcomprt.branch="${BRANCH}"
LABEL tech.cavcrosby.debcomprt.commit="${COMMIT}"
LABEL tech.cavcrosby.debcomprt.vcs-repo="https://github.com/cavcrosby/debcomprt"

# create user that will build software from project
ARG USER_NAME="builder"
ARG group_name="${USER_NAME}"
ARG USER_ID="1000"
ARG GROUP_ID="1000"
ARG user_home="/home/${USER_NAME}"

ENV WORKING_DIR "/debcomprt/build"
WORKDIR "${WORKING_DIR}"

RUN apt-get update && apt-get install --assume-yes \
    build-essential \
    debhelper \
    devscripts \
    git \
    sudo

# In the event the base image does not contain a 'sh' binary in /usr/bin.
#
# Links the go tool to a directory that is in root's secure_path (sudoers file).
#
# MONITOR(cavcrosby): dpkg-shlibdeps associates a package version for libc6
# that does not currently exist in bulleye's standard repos. At this time, the
# binary control file inserts a libc6 version of 2.3.2/2.32 whereas bullseye
# repos only have up to 2.31/2.3.1. The dpkg (dynamic library to package
# versioning) 'symbols' file appears to be the first metadata used by
# dpkg-shlibdeps to determine the packaging version. I am unsure of the syntax
# used by this file, but a workaround was noticed when having dpkg-shlibdeps use
# the dpkg 'shlibs' file instead for libc. This appears only possible by removing
# libc's dpkg symbols file. This will need to be monitored in the event if the
# symbols file gets corrected over time.
RUN ln --symbolic --force /bin/sh /usr/bin/sh \
    && ln --symbolic --force /usr/local/go/bin/go /usr/bin/go \
    && rm /var/lib/dpkg/info/libc6:amd64.symbols

RUN groupadd --gid "${GROUP_ID}" "${group_name}" \
    && useradd --create-home --home-dir "${user_home}" --uid "${USER_ID}" --gid "${GROUP_ID}" --shell /bin/bash "${USER_NAME}" \
    && echo "Defaults       env_keep += \"DEBCOMPRT_VERSION\"" > "/etc/sudoers.d/${USER_NAME}" \
    && echo "${USER_NAME} ALL=(ALL:ALL) PASSWD: ALL, NOPASSWD: /debcomprt/build/debcomprt-*/debian/rules" >> "/etc/sudoers.d/${USER_NAME}"

USER "${USER_NAME}"
ENTRYPOINT ["/bin/bash", "-c", "cd ${EXTRACTED_UPSTREAM_TARBALL} && make setup && debuild --preserve-envvar='DEBCOMPRT_VERSION' --rootcmd=sudo --unsigned-source --unsigned-changes"]
