There are a variety of ways to deploy the system:

1. As a primitive provided by Zos
2. Using cli tools and scripts
3. Via a Pulumi based deployer found in this repo

These have various tradeoffs that are explored below.

## Zos Primitive

The Zos primitive provides a convenient way to deploy Quantum Safe Storage but it isn't very flexible. It's not possible to access the zstor process and certain functions like recovering to a new frontend machine are not available.

This method could be useful for basic testing. How to deploy the primitive version using Terraform is [covered in the manual](https://manual.grid.tf/documentation/system_administrators/terraform/resources/terraform_qsfs.html).

## Cli Tools and Scripts

Using ThreeFold's `tfcmd` cli tool, it's possible to deploy all elements needed for Quantum Safe Storage. This method offers the most flexibility and control over the deployment process. An example for how to do this is included in a later section of these docs.

## Pulumi Deployer

The Pulumi deployer is [included](https://github.com/threefoldtech/quantum-storage/tree/master/pulumi) in this repo. It offers a largely automated process that covers advanced use cases like replacing failed backends and recovering to a new frontend machine.
