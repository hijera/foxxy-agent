# Android phones as swarm nodes



**Goal.** A phone that runs FoxxyCode in Termux joins a relay you run on a laptop or a server, and you drive it from the relay's web UI or from the console like any other node: prompts, tools that run on the phone, permission prompts, the phone's own sessions. The phone opens the connection itself, so it needs no public address, no open port and no VPN; a phone on a mobile network behind the carrier's NAT joins the same way as one on your Wi-Fi.

**Environment.** A machine for the relay with FoxxyCode 1.2.31 or later: a laptop on the same Wi-Fi, or a server with a public address. An Android phone with [Termux](https://termux.dev), 64-bit, Android 7.0 or later. A model the phone can reach and a key for it: the phone runs the model calls and the tools itself, the relay only carries the traffic. The background is in [Android (Termux)](../getting-started/android.md) and [Swarm](../operate/swarm.md); the shape of a relay with nodes is the one of [A relay and its nodes in Docker](swarm-relay-and-nodes.md).

## 1. The relay

Two tokens decide who may use the relay and who may join it ([Security](../operate/swarm.md#security)). Mint them on the relay's machine:

```bash
mkdir -p ~/.foxxycode
printf 'SWARM_CLIENT_TOKEN=%s\nSWARM_PAIRING_TOKEN=%s\n' "$(openssl rand -hex 16)" "$(openssl rand -hex 16)" >> ~/.foxxycode/.env
chmod 600 ~/.foxxycode/.env
```

`~/.foxxycode/config.yaml` of the relay runs nothing but the relay:

```yaml
httpserver:
  enable: false

swarm:
  enable: true
  host: "0.0.0.0"
  port: 12346
  name: "home"
  auth_token: "${SWARM_CLIENT_TOKEN}"
  pairing_tokens: ["${SWARM_PAIRING_TOKEN}"]
```

```bash
foxxycode serve
```

The relay listens on every address of the machine, so the phone reaches it at the machine's address on the Wi-Fi, for example `http://192.168.1.10:12346`. From the phone's browser, `http://192.168.1.10:12346/swarm/info` answers without a token and says the relay is up.

A relay on a server the phone reaches over the internet takes a certificate (`swarm.tls`, [Encryption and proxies](../operate/swarm.md#encryption-and-proxies)) and the phone joins `https://...`: the tokens travel in the request headers, and a mobile network may pass plain HTTP through a proxy that re-frames it, which the tunnel does not survive ([Two transports](../operate/swarm.md#two-transports)).

## 2. FoxxyCode in Termux

On the phone, in Termux:

```bash
pkg install curl tmux
```

Install the Android archive from [FoxxyCode releases](https://github.com/hijera/foxxy-agent/releases), following [the Termux installation steps](../getting-started/android.md#install).

After installation, reload `~/.bashrc` and check `foxxycode -v`.

## 3. The phone's configuration

The phone is an ordinary `foxxycode serve` with a model, a token on its own API and one `swarm.join` entry. There is no `advertise_url`, so the phone dials the relay and the relay drives it back down that connection (the tunnel transport). `~/.foxxycode/config.yaml` in Termux:

```yaml
providers:
  - name: openai
    type: openai
    api_key: "${OPENAI_API_KEY}"

models:
  - model: "openai/gpt-5.6-terra"

agent:
  model: "openai/gpt-5.6-terra"

httpserver:
  enable: true
  auth_token: "${NODE_TOKEN}"

swarm:
  join:
    - url: "http://192.168.1.10:12346"
      name: "pixel"
      pairing_token: "${SWARM_PAIRING_TOKEN}"
      token: "${NODE_TOKEN}"
```

The secrets go into `~/.foxxycode/.env` next to it: the model key, the pairing token of the relay and a token of the phone's own, which the relay presents when it forwards a request to the phone.

```bash
cat >> ~/.foxxycode/.env <<'EOF'
OPENAI_API_KEY=sk-...
SWARM_PAIRING_TOKEN=<the relay's pairing token>
NODE_TOKEN=<a token for this phone>
EOF
chmod 600 ~/.foxxycode/.env
```

Any provider from [Configuration](../getting-started/configuration.md) works the same way. `name` becomes the phone's path on the relay, so give every phone its own.

Before starting anything, the dry run checks the file, the model and the relay:

```bash
foxxycode serve --dry-run
```

## 4. Start it and keep it running

```bash
termux-wake-lock
tmux new -s foxxycode 'foxxycode serve'
```

Detach with **Ctrl+B**, then **D**; `tmux attach -t foxxycode` brings the log back. `foxxycode serve --daemon` is the alternative without tmux ([foxxycode serve](../operate/serve.md)).

Within a few seconds of the start the phone's log says `swarm: joining relay` with `transport=tunnel`, and the relay's log says `swarm node registered` and `swarm tunnel established` for `pixel`. On the relay's machine:

```bash
export T=<the relay's client token>
curl -s -H "Authorization: Bearer $T" http://127.0.0.1:12346/swarm/nodes
```

The list names `pixel` with `"transport": "tunnel"` and `"online": true`.

Android stops the processes of an app it considers idle. The wake lock keeps Termux running with the screen off; also let Termux run unrestricted in the battery settings, and on Android 12 and later lift the limit on background processes that Termux's documentation describes ([Running foxxycode serve](../getting-started/android.md#running-foxxycode-serve)).

## 5. Drive the phone

**The browser.** Open the relay, `http://192.168.1.10:12346/`. The page says the relay needs a token: open the environment chip in its header, **Connect to…**, and give it a name, the relay's address and the client token, then **Connect**. The swarm map shows `pixel` under the relay, marked as a node that dials out.


*The relay `home` with the phone `pixel` under it; the dotted line is the connection the phone opened.*

Click `pixel`, and the History drawer, the composer and the settings are the phone's: the environment chip reads `pixel` and the folder is the Termux home. A turn started there runs on the phone: its model calls, its commands in Termux's `bash`, its files. The permission prompts come to you, in the browser.


*Asked which device it runs on, the agent wants to run `getprop` in Termux's `bash` on the phone, and the approval is asked in the browser.*

**The console.** The phone's mount is a base URL like any other remote ([Working with remote nodes](swarm-remote-nodes.md)):

```bash
foxxycode --remote http://192.168.1.10:12346/swarm/nodes/pixel --remote-token "$T"
foxxycode -p "Which Android version is this? Use getprop." --remote http://192.168.1.10:12346/swarm/nodes/pixel --remote-token "$T"
```

The phone's workspace is the Termux home, where `foxxycode serve` was started. The phone's shared storage (downloads, photos) becomes reachable after `termux-setup-storage`, which asks Android for the permission once; a session's folder can then point under `~/storage/shared`.

## What tends to go wrong

- **The relay exits at startup with a message about binding off loopback without a client token.** `SWARM_CLIENT_TOKEN` did not reach it: the file is `~/.foxxycode/.env` of the account that runs the relay.
- **The phone never appears.** Its pairing token is not one of the relay's `pairing_tokens`, or the phone cannot reach the address: `curl http://192.168.1.10:12346/swarm/info` in Termux answers when the network is right. A guest Wi-Fi often keeps devices apart.
- **The phone drops out when the screen goes off.** The wake lock is missing, or the battery settings still restrict Termux; the relay marks the node offline once its lease runs out, and it returns by itself when the process runs again.
- **A request through the phone's mount answers 401.** The `token` in `swarm.join` is not the phone's `httpserver.auth_token`; both read `NODE_TOKEN` in the configuration above for that reason.
- **`swarm tunnel established` never follows `swarm node registered` on a mobile network.** Something between the phone and the relay re-frames HTTP. Give the relay TLS and join `https://...`.
- **Two phones claim one name.** Only the first keeps it: the relay proves a name with a per-lease secret, not with the shared pairing token. Name every phone differently.
