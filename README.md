Kubernetes Port-Forward Manager (kpfm) is an utility to manage multiple port-forwards to Kubernetes environments.

It's useful for developers who need to open multiple port-forwards to remote services during local development.

Features:
- Collection of services per kube context.
- Context aware. If your kube context changes the PF are redirected to the new cluster.
- PF health aware. If a PF fails, it is reconnected.

Usage:
- Clone the repository
- run `make install`
- Add your config to the `~/.config/kpfm/config.yaml` file. Check the [sample](./sample/config.yml) file for the expected structure.