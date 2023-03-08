# Troubleshooting

## Only zstor is started, not the local zdb and 0-db-fs

Verify: execute `zinit list` in a shell minside the running container.
The output will look like

```log
ns: Blocked
wait_hooks: Blocked
fs: Blocked
zstor: Spawned
zdb: Blocked
```

Check:

```sh
cd /var/run
/run # ls
```

If there is no `zstor.sock`, zstor has not created a communication socket yet so the test command in the zinit configuration fails.

Check `/data/zstor.log` for further troubleshooting
