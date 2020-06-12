#!/usr/bin/env bash

shopt -s extglob
set -o errtrace
set -o errexit

DIADEM_HOME=/opt/diadem
PROTOBUF_VERSION=3.5.1

log()  { printf "%b\n" "$*"; }
debug(){ [[ ${diadem_debug_flag:-0} -eq 0 ]] || printf "%b\n" "$*" >&2; }
fail() { log "\nERROR: $*\n" >&2 ; exit 1 ; }

diadem_install_initialize()
{
  if [ ! -d ${DIADEM_HOME} ] || [ ! -d ${DIADEM_HOME}/bin ] || [ ! -d ${DIADEM_HOME}/workdir ]; then
    log "Creating directory ${DIADEM_HOME} ${DIADEM_HOME}/bin ${DIADEM_HOME}/workdir"
    sudo mkdir -p ${DIADEM_HOME} ${DIADEM_HOME}/bin ${DIADEM_HOME}/workdir
    sudo chown -R ${USER} ${DIADEM_HOME}
  else
    log "${DIADEM_HOME} ${DIADEM_HOME}/bin ${DIADEM_HOME}/workdir already exists."
  fi
}

diadem_detect_os()
{
  log "Detecting operating system..."
  if grep -i Microsoft /proc/version >/dev/null 2>&1; then
    platform=linux
    os_type=windows
  elif grep -i ubuntu /proc/version >/dev/null 2>&1; then
    platform=linux
    os_type=ubuntu
  elif uname | grep -i darwin >/dev/null 2>&1; then
    platform=osx
    os_type=darwin
  else
    fail "Unable to detect OS..."
  fi
  log "Found ${platform} on ${os_type}."
}

diadem_install_commands_setup()
{
  brew_installed=false

  \which which >/dev/null 2>&1 || fail "Could not find 'which' command, make sure it's available first before continuing installation."
  \which grep >/dev/null 2>&1 || fail "Could not find 'grep' command, make sure it's available first before continuing installation."
  \which unzip >/dev/null 2>&1 || fail "Could not find 'unzip' command, make sure it's available first before continuing installation."

  if \which curlx >/dev/null 2>&1; then
    download_command="curl -sL -o"
  elif \which wget >/dev/null 2>&1; then
    download_command="wget -q -O"
  fi

  if \which brew >/dev/null 2>&1; then
    brew_installed=true
  fi
}

diadem_install_dependencies()
{
  protobuf_satisfied=false
  if \which protoc >/dev/null 2>&1; then
    PROTOBUF_VERSION_INSTALLED=$(protoc --version | awk '{print $2}')
    log "Detected protobuf version ${PROTOBUF_VERSION_INSTALLED}"
    if [ "${PROTOBUF_VERSION_INSTALLED}" != "${PROTOBUF_VERSION}" ]; then
      log "Protobuf version ${PROTOBUF_VERSION_INSTALLED} does not match required version ${PROTOBUF_VERSION}"
    else
      protobuf_satisfied=true
    fi
  fi

  if ! \which protoc >/dev/null 2>&1 || ! $protobuf_satisfied; then
    log "Installing protobuf version ${PROTOBUF_VERSION}..."
    if $brew_installed; then
      brew install protobuf
    else
	    $download_command /tmp/protoc-${PROTOBUF_VERSION}-${platform}-x86_64.zip \
        https://github.com/google/protobuf/releases/download/v${PROTOBUF_VERSION}/protoc-${PROTOBUF_VERSION}-${platform}-x86_64.zip
	    sudo unzip /tmp/protoc-${PROTOBUF_VERSION}-${platform}-x86_64.zip -d /usr/local
      sudo chmod 755 /usr/local/bin/protoc
	    sudo find /usr/local/include/google -type d -exec chmod 755 -- {} +
      sudo find /usr/local/include/google -type f -exec chmod 644 -- {} +
	    rm /tmp/protoc-${PROTOBUF_VERSION}-${platform}-x86_64.zip
    fi
  fi

  if ! \which protoc >/dev/null 2>&1; then
    fail "protobuf installation failed"
  fi
}

diadem_download_executable()
{
  if $brew_installed; then
    log "Installing diadem using homebrew..."
    brew tap diademnetwork/client
    brew install diadem
  else
    echo "Downloading diadem executable..."
    $download_command ${DIADEM_HOME}/bin/diadem https://private.delegatecall.com/diadem/${platform}/latest/diadem
    chmod +x ${DIADEM_HOME}/bin/diadem
  fi
  if [ ! -h "/usr/local/bin/diadem" ]; then
    sudo rm -f "/usr/local/bin/diadem"
    sudo ln -s ${DIADEM_HOME}/bin/diadem /usr/local/bin/diadem
  fi
}

diadem_configure()
{
  cd ${DIADEM_HOME}/workdir
  log "Running diadem init in ${DIADEM_HOME}/workdir"
  diadem init

  log "Creating genesis.json in ${DIADEM_HOME}/workdir"
  cat > ${DIADEM_HOME}/workdir/genesis.json <<EOF
{
    "contracts": [
    ]
}
EOF

  log "Creating diadem.yml in ${DIADEM_HOME}/workdir"
  echo > ${DIADEM_HOME}/workdir/diadem.yml
}

diadem_create_startup()
{
  diadem_startup=""
  if [ "$os_type" = "windows" ] || [ "$os_type" = "darwin" ]; then
    log "Startup script for ${platform} on ${os_type} is currently unsupported"
    return
  fi

  if \which systemctl >/dev/null 2>&1; then
    log "Creating systemd startup script"
    cat > /tmp/diadem.service <<EOF
[Unit]
Description=Diadem
After=network.target

[Service]
Type=simple
User=${USER}
WorkingDirectory=${DIADEM_HOME}/workdir
ExecStart=/usr/local/bin/diadem run
Restart=always
RestartSec=2
StartLimitInterval=0
LimitNOFILE=500000
StandardOutput=syslog
StandardError=syslog

[Install]
WantedBy=multi-user.target
EOF
    sudo mv /tmp/diadem.service /etc/systemd/system/diadem.service
    sudo systemctl daemon-reload
    sudo systemctl start diadem.service
    diadem_startup=systemd
  else
    log "Startup script for ${platform} on ${os_type} is currently available only for systemd"
  fi
}

diadem_done()
{
  if [ "$diadem_startup" = "systemd" ]; then
    printf "%b" "
Startup script has been installed via systemd.

To view its status:

  \$ sudo systemctl status diadem.service

To enable it at startup:

  \$ sudo systemctl enable diadem.service

To view logs:

  \$ sudo journalctl -u diadem.service -f
"
  else
    printf "%b" "
To run diadem:

  \$ cd ${DIADEM_HOME}/workdir
  \$ diadem run
"
  fi
}

diadem_install()
{
  diadem_install_initialize
  diadem_detect_os
  diadem_install_commands_setup
  diadem_install_dependencies
  diadem_download_executable
  diadem_configure
  diadem_create_startup
  diadem_done
}

diadem_install "$@"
