#!/usr/bin/env bash

# Script to read .tool-versions library versions into environment variables
# in the form of LIBRARY_NAME_VERSION=VERSION_NUMBER

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
ENV_FILE=${SCRIPT_DIR}/../.env

# Change to the repository base.
cd ${SCRIPT_DIR}/../

# First argument is a boolean of 0 = false or 1 = true to echo the variable.
# Second argument is the string to echo to std out.
to_echo() {
    if [ $1 -eq 1 ]
    then
        echo "$2"
    fi
}

# First argument is the boolean of 0 = false or 1 = true to echo what is parsed to std out.
read_tool_versions_write_to_env() {
    # clear the env file before writing to it later
    echo "" > ${ENV_FILE}
    # loop over each line of the .tool-versions file
    while read line; do
        to_echo ${1} "Original line: $line"
        # split the line into a bash array using the default space delimeter
        lineArray=($line)
        # get the key and value from the array, set the key to all uppercase
        key=${lineArray[0]^^}
        value=${lineArray[1]}
        # ignore comments, comments always start with #
        if [[ ${key:0:1} != "#" ]]; 
        then
            to_echo ${1} "Parsed line:   ${key}_VERSION=${value}"
            # echo the variable to the .env file
            echo "${key}_VERSION=${value}" >> ${ENV_FILE}
        fi
    done <.tool-versions
}

# We have two use cases
# 1. In the make file we have commands that need just the version for a single library and
#    being able to just grab that single one inline is very helpful. For this we pass in the
#    library we want the version for and only its version will be sent to std out.
# 2. In CI we want to convert the tool-versions to an env file that can then be used by tools
#    like dotenv to read those into github actions outputs. For this we do not pass any arguments
#    to the script.
if [ $# -eq 1 ]
then
    # Use case 1
    # If argument is passed only echo the version number for the specified library to std out
    read_tool_versions_write_to_env 0
    # load the env file variables
    source ${ENV_FILE}
    # print the variable specified in stdin
    echo "${!1^^}"
else
    # Use case 2
    # If no argument is passed just echo all the read versions to std out, which is useful
    # if something goes wrong in CI.
    read_tool_versions_write_to_env 1
    echo "You can run 'source .env' to load these environment variables into your shell now."
fi
