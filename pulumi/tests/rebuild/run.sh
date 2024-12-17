#!/bin/bash

source ../../venv/bin/activate

../test-scripts-local/deploy.sh
../test-scripts-local/upload_remote_scripts.sh
../test-scripts-local/create_data.sh
../test-scripts-local/redeploy.sh
../test-scripts-local/recover.sh
../test-scripts-local/upload_remote_scripts.sh
../test-scripts-local/remove_and_rebuild.sh
../test-scripts-local/destroy.sh
./cleanup.sh
