#!/usr/bin/env python3
"""Action1 helper for the DadLAN LLM host (JParrisDesktop by default).

Consolidates several one-off debugging scripts (key fetch, process check,
directory listing, connectivity test) into one CLI. Credentials are never
hardcoded: they load at runtime from the Action1 config file below, which
must stay 0600.

Requires the local `action1_client` module from ~/dev/action1/fedora.

Usage:
    python3 tools/action1_llm_host.py fetch-key
    python3 tools/action1_llm_host.py ps
    python3 tools/action1_llm_host.py llm-proc
    python3 tools/action1_llm_host.py list-dir
    python3 tools/action1_llm_host.py test
    python3 tools/action1_llm_host.py run "<powershell script>" --name "Ad-hoc"
"""
import argparse
import json
import os
import pathlib
import sys

ACTION1_CLIENT_PATH = os.environ.get(
    "ACTION1_CLIENT_PATH", "/home/josh/dev/action1/fedora"
)
ACTION1_CONFIG_PATH = pathlib.Path(
    os.environ.get("ACTION1_CONFIG_PATH", str(pathlib.Path.home() / ".config" / "action1.json"))
)
ACTION1_REGION = os.environ.get("ACTION1_REGION", "Australia")

# Defaults target JParrisDesktop, the DadLAN LLM host. Override per-machine
# (e.g. for the Laptop02 canary) via ACTION1_ORG_ID / ACTION1_ENDPOINT_ID.
DEFAULT_ORG_ID = "d11f37bc-ada3-4680-82eb-fc96c295ec49"
DEFAULT_ENDPOINT_ID = "104c8bbe-000f-4f97-b6ef-6d089bdf00a9"

LLM_KEY_TOKEN_PATH = pathlib.Path(
    os.environ.get(
        "FORGEGRID_LLM_TOKEN_PATH",
        str(pathlib.Path.home() / ".config" / "forgegrid" / "coordinator" / "llm-token.txt"),
    )
)


def _client():
    sys.path.insert(0, ACTION1_CLIENT_PATH)
    from action1_client import Action1Client  # type: ignore

    if not ACTION1_CONFIG_PATH.exists():
        raise SystemExit(f"Action1 config not found at {ACTION1_CONFIG_PATH}")
    mode = ACTION1_CONFIG_PATH.stat().st_mode & 0o777
    if mode != 0o600:
        raise SystemExit(
            f"Refusing to load {ACTION1_CONFIG_PATH}: expected 0600 permissions, found {oct(mode)}"
        )
    creds = json.loads(ACTION1_CONFIG_PATH.read_text())
    c = Action1Client(ACTION1_REGION, **creds)
    c.authenticate()
    return c


def _run(client, script, name, org_id, ep_id):
    res = client.run_script(org_id, ep_id, script, name=name)
    client.wait_for_completion(org_id, res["id"])
    return client.endpoint_output(org_id, res["id"], ep_id)


def cmd_fetch_key(args):
    client = _client()
    out = _run(
        client,
        "Get-Content D:\\DadLAN\\local-llm-service\\api_key.txt",
        "Get LLM Key",
        args.org_id,
        args.endpoint_id,
    )
    key = out.get("raw_output", "").strip()
    if not key:
        print("NO KEY RETURNED:", out)
        raise SystemExit(1)
    LLM_KEY_TOKEN_PATH.parent.mkdir(parents=True, exist_ok=True)
    LLM_KEY_TOKEN_PATH.write_text(key)
    LLM_KEY_TOKEN_PATH.chmod(0o600)
    print(f"Saved key to {LLM_KEY_TOKEN_PATH} (0600)")


def cmd_ps(args):
    client = _client()
    out = _run(
        client,
        "Get-Process | Where-Object {$_.ProcessName -match 'ForgeGrid'} | "
        "Select-Object ProcessName, Id, CommandLine | ConvertTo-Json",
        "Check Processes",
        args.org_id,
        args.endpoint_id,
    )
    print(out.get("output", ""))


def cmd_llm_proc(args):
    client = _client()
    out = _run(
        client,
        "Get-WmiObject Win32_Process | Where-Object { $_.CommandLine -match '11435' -or "
        "$_.CommandLine -match 'llm' } | Select-Object ProcessName, CommandLine | ConvertTo-Json",
        "Get LLM Proc",
        args.org_id,
        args.endpoint_id,
    )
    print(out.get("output", ""))


def cmd_list_dir(args):
    client = _client()
    out = _run(
        client,
        "dir D:\\DadLAN\\local-llm-service | ConvertTo-Json",
        "List Dir",
        args.org_id,
        args.endpoint_id,
    )
    print(out.get("output", ""))


def cmd_test(args):
    client = _client()
    out = _run(client, "Write-Host 'HELLO WORLD'", "Test", args.org_id, args.endpoint_id)
    print(out)


def cmd_run(args):
    client = _client()
    out = _run(client, args.script, args.name, args.org_id, args.endpoint_id)
    print(out.get("output", out))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--org-id", dest="org_id", default=DEFAULT_ORG_ID)
    parser.add_argument("--endpoint-id", dest="endpoint_id", default=DEFAULT_ENDPOINT_ID)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("fetch-key").set_defaults(func=cmd_fetch_key)
    sub.add_parser("ps").set_defaults(func=cmd_ps)
    sub.add_parser("llm-proc").set_defaults(func=cmd_llm_proc)
    sub.add_parser("list-dir").set_defaults(func=cmd_list_dir)
    sub.add_parser("test").set_defaults(func=cmd_test)

    run_parser = sub.add_parser("run")
    run_parser.add_argument("script")
    run_parser.add_argument("--name", default="Ad-hoc")
    run_parser.set_defaults(func=cmd_run)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
