#!/bin/bash

NODE_VERSION=18

cd ../smoke || exit

echo "Switching to required Node.js version $NODE_VERSION..."
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
nvm use $NODE_VERSION

echo "Initializing soak test..."
terminated_by_script=false
while IFS= read -r line; do
    echo "$line"
    # Check if the line contains the target string
    if echo "$line" | grep -q "ocr2:inspect:responses"; then
        # Send SIGINT (Ctrl+C) to the 'go test' process
        sudo pkill -INT -P $$ go 2>/dev/null
        terminated_by_script=true
        break
    fi
done < <(sudo go test -timeout 24h -count=1 -run TestSolanaOCRV2Smoke/embedded -test.timeout 30m 2>&1)

# Capture the PID of the background process
READER_PID=$!

# Start a background timer (sleeps for 15 minutes, then sends SIGALRM to the script)
( sleep 900 && kill -s ALRM $$ ) &
TIMER_PID=$!

# Set a trap to catch the SIGALRM signal for timeout
trap 'on_timeout' ALRM

# Function to handle timeout
on_timeout() {
    echo "Error: failed to start soak test: timeout exceeded (15 minutes)."
    # Send SIGINT to the 'go test' process
    pkill -INT -P $$ go 2>/dev/null
    # Clean up
    kill "$TIMER_PID" 2>/dev/null
    kill "$READER_PID" 2>/dev/null
    exit 1
}

# Wait for the reader process to finish
wait "$READER_PID"
EXIT_STATUS=$?

# Clean up: kill the timer process if it's still running
kill "$TIMER_PID" 2>/dev/null

if [ "$terminated_by_script" = true ]; then
    echo "Soak test started successfully"
    exit 0
else
    echo "Soak test failed to start"
    exit 1
fi
