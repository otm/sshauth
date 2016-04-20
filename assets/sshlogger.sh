#!/bin/bash
# sshlogger

logger -p auth.info "SSH login: user=$1 bucket=$2 key=$3"

if [ -z "${SSH_ORIGINAL_COMMAND}" ]; then
	# No command, give pty to user
	${SHELL}
else
	# Execute command for user
	${SHELL} -c "${SSH_ORIGINAL_COMMAND}"
fi
exit
