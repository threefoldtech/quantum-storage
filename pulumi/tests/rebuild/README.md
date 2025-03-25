This is meant to be a fully automated test of the rebuild/repair system in Zstor.

It does these steps:

1. Deploy a QSFS using an original configuration
2. Write some data into the QSFS (random files)
3. Replace one of the original Zdbs with a new one on a different node (do this for both data and metadata)
4. Upload a new config file to the frontend VM and try to hot reload the config using SIGUSR1
5. Check the `status` output from zstor to see if some data has been written to the new backends

Perhaps a better test would be to force zstor to rebuild the data from the new backend, by blocking access to enough of the original backends that using the new backend is necessary to fulfill the required shard count to rebuild.

To use it, just:

```
./run.sh
```

This runs a set of scripts in the correct order. You can also run the scripts individually and inspect the state step by step.
