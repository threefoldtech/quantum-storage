# Deploy QSFS with Pulumi

This is a Pulumi deployment script in Python that fully automates the setup of a QSFS instance. The following steps are required to use this script:

1. Install Pulumi and Python on your system
2. Use Pip to install the Python dependencies
3. Copy and edit vars.py and zstor_config.base.toml

Only Linux and MacOS are supported. If you run Windows, I'd recommend equipping yourself with a WSL environment.

## Install Pulumi and Python

We won't cover the details here. Probably your system already has `python3`.

For Pulumi, check here: https://www.pulumi.com/docs/iac/download-install/

## Install Python dependencies

We need some Python packages to make this work. Using a venv is recommended.

```
python -m venv .venv
source .venv/bin/activate
pip install pulumi pulumi_random pulumi_command pulumi_threefold
```

## Prep config

Two config files are needed. Examples are included here. Copy the examples to the expected paths, then edit the files according to your needs.

```
cp vars.example.py vars.py
cp zstor_config.base.example.toml zstor_config.base.toml

$EDITOR vars.py
$EDITOR zstor_config.base.toml
```

## Deploy

Prior to using Pulumi, you need to login. There are some options here, which you can read about, but the simplest thing is to just use `--local`:

```
pulumi login --local
```

Now we can bring up the deployment. Create a stack when prompted with your name of choice.

```
pulumi up
```

If you want to destroy the deployment, bring it down like this:

```
pulumi down
```
