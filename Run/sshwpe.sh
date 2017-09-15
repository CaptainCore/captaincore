if [ -n "$2" ]; then
  ssh austin@anchor.host+$1@ssh.gcp-us-central1-farm-01.wpengine.io "cd sites/$1/ && $2"
else
  ssh austin@anchor.host+$1@ssh.gcp-us-central1-farm-01.wpengine.io
fi
