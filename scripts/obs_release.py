#!/usr/bin/env python3
"""Prepare, update, and verify an exact stable release in the author's OBS project."""

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
from typing import NamedTuple
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

from obs_verify import verify_build


REPOSITORY = "arumata/gswitch"
API = "https://api.opensuse.org"
ROOT = Path(__file__).resolve().parents[1]


class Target(NamedTuple):
    project: str
    package: str

    @property
    def source(self):
        return f'/source/{self.project}/{self.package}'

    @property
    def build(self):
        return f'/build/{self.project}/openSUSE_Tumbleweed/x86_64/{self.package}'


TARGETS = {'test': Target('home:arumata', 'gswitch-automation-test'),
           'main': Target('home:arumata', 'gswitch')}
TEST_TARGET = TARGETS['test']


def gh_api(path):
    for attempt in range(3):
        try:
            result = subprocess.run(
                ["gh", "api", f"repos/{REPOSITORY}/{path}"],
                capture_output=True, text=True, check=False, timeout=30,
            )
        except subprocess.TimeoutExpired:
            transient = True
        else:
            if result.returncode == 0:
                return json.loads(result.stdout)
            transient = any(text in result.stderr for text in (
                'EOF', 'connection reset', 'TLS handshake timeout',
                'HTTP 502', 'HTTP 503', 'HTTP 504',
            ))
        if not transient or attempt == 2:
            break
        time.sleep(attempt + 1)
    raise ValueError(f"GitHub API request failed for {path}; no OBS mutation attempted")


def release_identity(tag, commit=None, api=gh_api):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Only stable vMAJOR.MINOR.PATCH tags are allowed")
    if commit is not None and not re.fullmatch(r"[0-9a-f]{40}", commit):
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
    if (obj["type"] != "commit" or not re.fullmatch(r'[a-f0-9]{40}', obj['sha'])
            or (commit is not None and obj["sha"] != commit)):
        raise ValueError("Published tag does not resolve to the expected release commit")
    commit = obj['sha']
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
    def __init__(self, output=None):
        username = os.environ.get("OBS_USERNAME", "")
        password = os.environ.get("OBS_PASSWORD", "")
        if username != 'arumata' or not password:
            raise ValueError("OBS_USERNAME must be arumata and OBS_PASSWORD must be set")
        self.authorization = "Basic " + base64.b64encode(
            f"{username}:{password}".encode()).decode()
        self.opener = urllib.request.build_opener(NoRedirect())
        self.output = output

    def request(self, path, data=None):
        request = urllib.request.Request(API + path, data=data, headers={
            "Authorization": self.authorization, "Content-Type": "application/xml",
        }, method="PUT" if data is not None else "GET")
        try:
            with self.opener.open(request, timeout=60) as response:
                data = response.read(64 * 1024 * 1024 + 1)
                if len(data) > 64 * 1024 * 1024:
                    raise ValueError('OBS response exceeds the 64 MiB download limit')
                if self.output and path.endswith('?expand=1'):
                    (self.output / 'last-source-directory.xml').write_bytes(data)
                return data
        except (urllib.error.URLError, TimeoutError) as exc:
            # Never print response bodies, request objects, or credentials.
            status = exc.code if isinstance(exc, urllib.error.HTTPError) else "network error"
            raise ValueError(f"OBS request failed ({status}); inspect state before retrying") from None


def directory(obs, target=TEST_TARGET):
    root = ET.fromstring(obs.request(target.source + "?expand=1"))
    if root.tag != "directory" or not root.get("srcmd5"):
        raise ValueError("OBS returned an invalid source directory")
    return root


def service_at(obs, source, target=TEST_TARGET):
    return obs.request(target.source + "/_service?" + urllib.parse.urlencode({"rev": source.get("srcmd5")}))


def verify_sources(obs, source, wanted, identity, target=TEST_TARGET):
    info = source.find("serviceinfo")
    if info is None or info.get("code") != "succeeded":
        return None
    if service_at(obs, source, target) != wanted:
        raise ValueError("OBS recipe changed during verification")
    expected = {"_service", "gswitch.spec", "gswitch.changes",
                "_service:go_modules:vendor.tar.gz", "_service:obs_scm:gswitch.obsinfo",
                f"_service:obs_scm:gswitch-{identity['version']}.obscpio"}
    if {entry.get("name") for entry in source.findall("entry")} != expected:
        raise ValueError("Unexpected OBS source files; inspect stale or missing outputs")
    path = target.source + "/_service:obs_scm:gswitch.obsinfo?" + urllib.parse.urlencode({"rev": source.get("srcmd5")})
    text = obs.request(path).decode()
    fields = dict(line.split(": ", 1) for line in text.splitlines() if ": " in line)
    if fields.get("commit") != identity["commit"] or fields.get("version") != identity["version"]:
        raise ValueError("OBS generated metadata does not match the released commit and version")
    return {"revision": source.get("rev"), "srcmd5": source.get("srcmd5"),
            "service_status": "succeeded", "obsinfo": fields,
            "build_verified": False}


def update_sources(obs, wanted, identity, target=TEST_TARGET, timeout=1200, sleep=time.sleep, clock=time.monotonic):
    source = directory(obs, target)
    old = service_at(obs, source, target)
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
        obs.request(target.source + "/_service", wanted)
    deadline = clock() + timeout
    while clock() < deadline:
        source = directory(obs, target)
        if service_at(obs, source, target) != wanted:
            raise ValueError("OBS recipe does not match the requested release")
        state = source.find("serviceinfo")
        code = state.get("code") if state is not None else None
        if code in {"failed", "broken"}:
            raise ValueError("OBS source services failed; preserve the server log")
        verified = verify_sources(obs, source, wanted, identity, target)
        if verified:
            verified['source_write_performed'] = old != wanted
            return verified
        sleep(15)
    raise ValueError("OBS source preparation timed out; inspect before retrying")


def check_access(obs, target):
    metadata = ET.fromstring(obs.request(target.source + '/_meta'))
    if (metadata.tag != 'package' or metadata.get('name') != target.package
            or metadata.get('project') != target.project or metadata.find('scmsync') is not None):
        raise ValueError('Unexpected or SCM-managed OBS package; inspect before writing')
    source = directory(obs, target)
    service_at(obs, source, target)
    return {'read_access_verified': True, 'write_access_verified': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", help='Expected full release commit; resolved from the tag when omitted')
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument('--target', choices=TARGETS, required=True)
    parser.add_argument('--mode', choices=['prepare', 'check-access', 'verify', 'update'], default='prepare')
    args = parser.parse_args()
    if args.output.exists() and any(args.output.iterdir()):
        parser.error('Use a fresh output directory to preserve previous receipts')
    args.output.mkdir(parents=True, exist_ok=True)
    target = TARGETS[args.target]
    receipt = {'target': args.target, 'mode': args.mode, 'project': target.project,
               'package': target.package, 'state': 'preparing', 'build_verified': False}
    path = args.output / "receipt.json"

    def save():
        path.write_text(json.dumps(receipt, indent=2) + '\n')

    save()
    try:
        identity = release_identity(args.tag, args.commit)
        wanted = recipe(identity)
        (args.output / '_service').write_bytes(wanted)
        receipt.update(identity, service_sha256=hashlib.sha256(wanted).hexdigest(), state='prepared')
        save()
        if args.mode != 'prepare':
            obs = OBS(args.output)
            receipt.update(check_access(obs, target), state='access_checked')
            save()
            if args.mode in {'verify', 'update'}:
                if args.mode == 'update':
                    source = update_sources(obs, wanted, identity, target)
                    receipt['write_access_verified'] = source['source_write_performed']
                else:
                    source = verify_sources(obs, directory(obs, target), wanted, identity, target)
                    if source is None:
                        raise ValueError('Source services have not succeeded')
                receipt.update(source, state='sources_verified')
                save()
                verify_build(obs, target, source, identity, args.output, receipt, save)
    except (ValueError, ET.ParseError, KeyError, OSError, subprocess.SubprocessError) as error:
        receipt.update(state='failed_or_unverified', error=str(error) if isinstance(error, ValueError) else type(error).__name__)
        raise ValueError(receipt['error']) from None
    finally:
        save()
    print(json.dumps(receipt, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, ET.ParseError, KeyError) as error:
        print(f"OBS handoff failed: {error}", file=sys.stderr)
        sys.exit(1)
