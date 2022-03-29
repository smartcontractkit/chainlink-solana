#!/usr/bin/env bash

# Script to read .tool-versions library versions into environment variables
# in the form of LIBRARY_NAME_VERSION=VERSION_NUMBER

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
ENV_FILE=${SCRIPT_DIR}/../.env

cd ${SCRIPT_DIR}/../

# First argument is a boolean of 0 = false or 1 = true to echo the variable
# Second argument is the string to echo
to_echo() {
    if [ $1 -eq 1 ]
    then
        echo "$2"
    fi
}

# First argument is the boolean of 0 = false or 1 = true to echo
read_file() {
    echo "" > ${ENV_FILE}
    while read line; do
        to_echo ${1} "Original line: $line"
        lineArray=($line)
        key=${lineArray[0]^^}
        value=${lineArray[1]}
        # ignore comments
        if [[ ${key:0:1} != "#" ]]; 
        then 
            
            to_echo ${1} "Parsed line:   ${key}_VERSION=${value}"
            echo "${key}_VERSION=${value}" >> ${ENV_FILE}
        fi
    done <.tool-versions
}

if [ $# -eq 1 ]
then
    # If argument is passed only echo the version number for the specified library to std out
    read_file 0
    source ${ENV_FILE}
    echo "${!1^^}"
else
    # If no argument is passed just echo all the read in versions to std out
    read_file 1
    echo "Run 'source .env' to load environment variables into your shell"
fi
