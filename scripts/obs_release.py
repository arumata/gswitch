#!/usr/bin/env python3
"""Prepare an exact stable-release OBS recipe; update only the opt-in test package."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET


REPOSITORY = "arumata/gswitch"
PROJECT = "home:arumata"
PACKAGE = "gswitch-automation-test"
API = "https://api.opensuse.org"
SOURCE = f"/source/{PROJECT}/{PACKAGE}"
ROOT = Path(__file__).resolve().parents[1]


def gh_api(path):
    result = subprocess.run(
        ["gh", "api", f"repos/{REPOSITORY}/{path}"],
        capture_output=True, text=True, check=False,
    )
    if result.returncode:
        raise ValueError("GitHub API request failed; no OBS mutation attempted")
    return json.loads(result.stdout)


def release_identity(tag, commit, api=gh_api):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Only stable vMAJOR.MINOR.PATCH tags are allowed")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("Expected the full release commit SHA")
    release = api(f"releases/tags/{tag}")
    if (release.get("draft") is not False or release.get("prerelease") is not False
            or not release.get("published_at") or release.get("tag_name") != tag):
        raise ValueError("Tag does not have a published stable GitHub release")
    latest = api("releases/latest")
    if latest.get("id") != release.get("id") or latest.get("tag_name") != tag:
        raise ValueError("Refusing an older release or rerun after a newer release")
    obj = api(f"git/ref/tags/{tag}")["object"]
    for _ in range(8):
        if obj["type"] != "tag":
            break
        obj = api(f"git/tags/{obj['sha']}")["object"]
    if obj["type"] != "commit" or obj["sha"] != commit:
        raise ValueError("Published tag does not resolve to the expected release commit")
    return {"repository": REPOSITORY, "tag": tag, "commit": commit,
            "version": tag[1:], "release_id": release["id"],
            "release_url": release["html_url"], "published_at": release["published_at"]}


def recipe(identity):
    root = ET.parse(ROOT / "packaging/opensuse/_service.release").getroot()
    root.find("service/param[@name='revision']").text = identity["commit"]
    root.find("service/param[@name='versionformat']").text = identity["version"]
    ET.indent(root, space="  ")
    return ET.tostring(root, encoding="utf-8") + b"\n"


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError("OBS redirect refused; inspect server state before retrying")


class OBS:
    def __init__(self):
        username = os.environ.get("OBS_USERNAME", "")
        password = os.environ.get("OBS_PASSWORD", "")
        if not username or not password or ":" in username:
            raise ValueError("OBS_USERNAME and OBS_PASSWORD are required for source writes")
        self.authorization = "Basic " + base64.b64encode(
            f"{username}:{password}".encode()).decode()
        self.opener = urllib.request.build_opener(NoRedirect())

    def request(self, path, data=None):
        request = urllib.request.Request(API + path, data=data, headers={
            "Authorization": self.authorization, "Content-Type": "application/xml",
        }, method="PUT" if data is not None else "GET")
        try:
            with self.opener.open(request, timeout=60) as response:
                return response.read()
        except (urllib.error.URLError, TimeoutError) as exc:
            # Never print response bodies, request objects, or credentials.
            status = exc.code if isinstance(exc, urllib.error.HTTPError) else "network error"
            raise ValueError(f"OBS request failed ({status}); inspect state before retrying") from None


def directory(obs):
    root = ET.fromstring(obs.request(SOURCE + "?expand=1"))
    if root.tag != "directory" or not root.get("srcmd5"):
        raise ValueError("OBS returned an invalid source directory")
    return root


def service_at(obs, source):
    return obs.request(SOURCE + "/_service?" + urllib.parse.urlencode({"rev": source.get("srcmd5")}))


def verify_sources(obs, source, wanted, identity):
    info = source.find("serviceinfo")
    if info is None or info.get("code") != "succeeded":
        return None
    if service_at(obs, source) != wanted:
        raise ValueError("OBS recipe changed during verification")
    expected = {"_service", "gswitch.spec", "gswitch.changes",
                "_service:go_modules:vendor.tar.gz", "_service:obs_scm:gswitch.obsinfo",
                f"_service:obs_scm:gswitch-{identity['version']}.obscpio"}
    if {entry.get("name") for entry in source.findall("entry")} != expected:
        raise ValueError("Unexpected OBS source files; inspect stale or missing outputs")
    path = SOURCE + "/_service:obs_scm:gswitch.obsinfo?" + urllib.parse.urlencode({"rev": source.get("srcmd5")})
    text = obs.request(path).decode()
    fields = dict(line.split(": ", 1) for line in text.splitlines() if ": " in line)
    if fields.get("commit") != identity["commit"] or fields.get("version") != identity["version"]:
        raise ValueError("OBS generated metadata does not match the released commit and version")
    return {"revision": source.get("rev"), "srcmd5": source.get("srcmd5"),
            "service_status": "succeeded", "obsinfo": fields,
            "build_verified": False}


def apply(obs, wanted, identity, timeout=1200, sleep=time.sleep, clock=time.monotonic):
    source = directory(obs)
    old = service_at(obs, source)
    state = source.find("serviceinfo")
    if state is not None and state.get("code") in {"running", "scheduled"}:
        raise ValueError("OBS services are busy; inspect the active run before retrying")
    if old != wanted:
        params = ET.fromstring(old).find("service[@name='obs_scm']")
        if params is None or params.findtext("param[@name='url']") != f"https://github.com/{REPOSITORY}.git":
            raise ValueError("Unexpected existing OBS source recipe")
        previous = params.findtext("param[@name='versionformat']", "")
        if (previous == identity['version']
                and params.findtext("param[@name='revision']") != identity['commit']):
            raise ValueError("Refusing a different commit for the same release version")
        if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", previous):
            if tuple(map(int, previous.split('.'))) > tuple(map(int, identity['version'].split('.'))):
                raise ValueError("Refusing to downgrade the OBS recipe")
        # One file commit starts server services. No redundant runservice POST.
        # Only this fixed experimental package is writable through this helper.
        obs.request(SOURCE + "/_service", wanted)
    deadline = clock() + timeout
    while clock() < deadline:
        source = directory(obs)
        if service_at(obs, source) != wanted:
            raise ValueError("OBS recipe does not match the requested release")
        state = source.find("serviceinfo")
        code = state.get("code") if state is not None else None
        if code in {"failed", "broken"}:
            raise ValueError("OBS source services failed; preserve the server log")
        verified = verify_sources(obs, source, wanted, identity)
        if verified:
            return verified
        sleep(15)
    raise ValueError("OBS source preparation timed out; inspect before retrying")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--apply", action="store_true", help="Write only the fixed OBS test package")
    args = parser.parse_args()
    identity = release_identity(args.tag, args.commit)
    wanted = recipe(identity)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / "_service").write_bytes(wanted)
    receipt = {**identity, "project": PROJECT, "package": PACKAGE,
               "service_sha256": hashlib.sha256(wanted).hexdigest(),
               "state": "prepared", "build_verified": False}
    path = args.output / "receipt.json"
    path.write_text(json.dumps(receipt, indent=2) + "\n")
    if args.apply:
        try:
            receipt.update(apply(OBS(), wanted, identity))
            receipt["state"] = "sources_verified"
        except (ValueError, ET.ParseError, KeyError) as exc:
            receipt.update(state="failed_or_unverified", error=str(exc))
            raise
        finally:
            path.write_text(json.dumps(receipt, indent=2) + "\n")
    print(json.dumps(receipt, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, ET.ParseError, KeyError) as error:
        print(f"OBS handoff failed: {error}", file=sys.stderr)
        sys.exit(1)
