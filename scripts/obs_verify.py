"""Verify a completed OBS build and its signed RPM in an isolated container."""

import hashlib
from pathlib import Path
import re
import subprocess
import time
import urllib.parse
import uuid
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1]
TUMBLEWEED = 'registry.opensuse.org/opensuse/tumbleweed@sha256:87eb3e69082bee2629fd71460c2bc015893b4bd045e9de098213cd406fdbb9ef'
# Independently verified home:arumata key, fingerprint 91510B31F8DBD52EA77917D4397732707BD9B6BD.
SIGNING_KEY_SHA256 = '6b0eb14b53ae55bd0f01993d9e4d88fba9bafde07d01ffd482550c02637d6967'


def rpmlint_result(log):
    summary = re.search(r'; (\d+) errors, (\d+) warnings,', log)
    if not summary:
        raise ValueError('rpmlint summary is missing')
    findings = [line for line in log.splitlines() if re.search(r': [EW]: ', line)]
    errors = [line for line in findings if ': E: ' in line]
    if len(errors) != int(summary[1]) or len(findings) - len(errors) != int(summary[2]):
        raise ValueError('rpmlint summary does not match its findings')
    pending = [line for line in errors if re.fullmatch(
        r'gswitch\.x86_64: E: polkit-untracked-privilege \(Badness: 10\) '
        r'com\.github\.arumata\.gswitch\.write-config \(auth_admin:auth_admin:auth_admin_keep\)', line)]
    filtered = re.search(r', (\d+) filtered,', log)
    return {'findings': findings, 'pending_distribution_review': pending,
            'unexpected_errors': [line for line in errors if line not in pending],
            'clean': not findings, 'filtered_count': int(filtered[1]) if filtered else None}


def save_response(obs, path, out, name):
    data = obs.request(path)
    (out / name).write_bytes(data)
    return data


def build_state(obs, target, out):
    query = urllib.parse.urlencode({'package': target.package,
                                   'repository': 'openSUSE_Tumbleweed', 'arch': 'x86_64'})
    result = ET.fromstring(save_response(obs, f'/build/{target.project}/_result?{query}', out, 'build-result.xml'))
    history = ET.fromstring(save_response(obs, target.build + '/_history', out, 'build-history.xml'))
    results = result.findall('result')
    if (len(results) != 1 or results[0].get('project') != target.project
            or results[0].get('repository') != 'openSUSE_Tumbleweed'
            or results[0].get('arch') != 'x86_64'):
        raise ValueError('Unexpected OBS build target in result')
    statuses = results[0].findall('status')
    if len(statuses) != 1 or statuses[0].get('package') != target.package:
        raise ValueError('Unexpected OBS package build status')
    if history.tag != 'buildhistory':
        raise ValueError('Invalid OBS build history')
    entries = history.findall('entry')
    return statuses[0].get('code'), dict(entries[-1].attrib) if entries else None


def require_source(obs, target, srcmd5):
    current = ET.fromstring(obs.request(target.source + '?expand=1'))
    if current.tag != 'directory' or current.get('srcmd5') != srcmd5:
        raise ValueError('OBS source revision changed during build verification')


def wait_build(obs, target, source, identity, out, timeout=1800,
               sleep=time.sleep, clock=time.monotonic):
    deadline = clock() + timeout
    while clock() < deadline:
        require_source(obs, target, source['srcmd5'])
        code, latest = build_state(obs, target, out)
        if code in {'failed', 'broken', 'unresolvable', 'disabled', 'excluded'}:
            raise ValueError(f'OBS build cannot be accepted: {code}')
        if code == 'succeeded' and latest and latest.get('srcmd5') == source['srcmd5']:
            if (not re.fullmatch(re.escape(identity['version']) + r'-[0-9]+', latest.get('versrel', ''))
                    or not re.fullmatch(r'[1-9][0-9]*', latest.get('bcnt', ''))):
                raise ValueError('OBS completed build has an unexpected version or build count')
            return latest
        sleep(15)
    raise ValueError('OBS build timed out; preserve status/history and inspect before retrying')


def file_entries(xml, element):
    entries = []
    seen = set()
    for entry in xml.findall(element):
        name = entry.get('name') if element == 'entry' else entry.get('filename')
        if (not name or '/' in name or name in {'.', '..'} or '\\' in name
                or any(ord(c) < 32 for c in name) or name in seen):
            raise ValueError('Unsafe or duplicate OBS artifact name')
        seen.add(name)
        entries.append(dict(entry.attrib, name=name))
    return entries


def collect(obs, target, source, build, out, files, checkpoint):
    raw = save_response(obs, target.source + '?' + urllib.parse.urlencode({'expand': 1, 'rev': source['srcmd5']}), out, 'source-directory.xml')
    source_xml = ET.fromstring(raw)
    if source_xml.get('srcmd5') != source['srcmd5']:
        raise ValueError('Source download snapshot does not match verified revision')
    sources = file_entries(source_xml, 'entry')
    binary_xml = ET.fromstring(save_response(obs, target.build, out, 'binary-list.xml'))
    if binary_xml.tag != 'binarylist':
        raise ValueError('Invalid OBS binary list')
    binaries = file_entries(binary_xml, 'binary')
    expected = {f"gswitch-{build['versrel']}.{build['bcnt']}.{arch}.rpm" for arch in ['src', 'x86_64']}
    if {e['name'] for e in binaries if e['name'].endswith('.rpm')} != expected:
        raise ValueError('OBS binary list does not match the completed build')
    for group, entries, prefix in [('sources', sources, target.source), ('binaries', binaries, target.build)]:
        directory = out / group
        directory.mkdir()
        for entry in entries:
            url = prefix + '/' + urllib.parse.quote(entry['name'], safe='')
            if group == 'sources':
                url += '?' + urllib.parse.urlencode({'rev': source['srcmd5']})
            data = obs.request(url)
            if int(entry.get('size', len(data))) != len(data):
                raise ValueError('OBS artifact size changed during download')
            if 'md5' in entry and hashlib.md5(data, usedforsecurity=False).hexdigest() != entry['md5']:
                raise ValueError('OBS source artifact digest mismatch')
            (directory / entry['name']).write_bytes(data)
            files[group].append({'name': entry['name'], 'bytes': len(data),
                                 'sha256': hashlib.sha256(data).hexdigest()})
            checkpoint()
    key = save_response(obs, f'/source/{target.project}/_pubkey', out, 'obs-public-key.asc')
    if hashlib.sha256(key).hexdigest() != SIGNING_KEY_SHA256:
        raise ValueError('OBS signing key changed; independently verify rotation before updating the pinned key')
    return files


def install(out, target, source, build, identity, run=subprocess.run):
    rpm_name = f"gswitch-{build['versrel']}.{build['bcnt']}.x86_64.rpm"
    disturl = f"obs://build.opensuse.org/{target.project}/openSUSE_Tumbleweed/{source['srcmd5']}-{target.package}"
    container_name = 'gswitch-obs-verify-' + uuid.uuid4().hex
    command = ['docker', 'run', '--rm', '--name', container_name, '--network', 'bridge', '--security-opt=no-new-privileges',
               '--cpus=4', '--memory=8g', '-v', f'{out.resolve()}:/work:ro',
               '-v', f'{ROOT / "scripts/obs_verify_rpm.sh"}:/verify.sh:ro',
               '-e', f'GSWITCH_EXPECTED_VERSION={identity["version"]}',
               '-e', f'GSWITCH_EXPECTED_DISTURL={disturl}', TUMBLEWEED,
               'sh', '/verify.sh', f'/work/binaries/{rpm_name}', '/work/obs-public-key.asc']
    try:
        with (out / 'install.log').open('w') as log:
            result = run(command, stdout=log, stderr=subprocess.STDOUT, timeout=900, check=False)
    finally:
        run(['docker', 'rm', '--force', container_name], capture_output=True, timeout=30, check=False)
    if result.returncode:
        raise ValueError('Signed RPM installation verification failed; see install.log')
    log = (out / 'install.log').read_text()
    hashes = dict((name, digest) for digest, name in re.findall(r'^([a-f0-9]{64})  /usr/bin/(gswitch(?:-tray)?)$', log, re.M))
    if set(hashes) != {'gswitch', 'gswitch-tray'}:
        raise ValueError('Installed binary hashes are missing from the verification log')
    return {'image': TUMBLEWEED, 'binary_sha256': hashes, 'signature_verified': True,
            'installed': True, 'reinstalled': True, 'removed_with_config_preserved': True,
            'graphical_runtime_verified': False, 'expected_disturl': disturl}


def verify_build(obs, target, source, identity, out, receipt, save):
    try:
        build = wait_build(obs, target, source, identity, out)
        save_response(obs, target.build + '/_log', out, 'build.log')
        receipt.update(build=build, state='build_completed')
        save()
        receipt['files'] = {'sources': [], 'binaries': []}
        collect(obs, target, source, build, out, receipt['files'], save)
        receipt['rpmlint'] = rpmlint_result((out / 'binaries/rpmlint.log').read_text())
        if receipt['rpmlint']['unexpected_errors']:
            save()
            raise ValueError('Unexpected rpmlint errors; inspect the complete findings')
        receipt.update(state='artifacts_downloaded')
        save()
        require_source(obs, target, source['srcmd5'])
        code, after = build_state(obs, target, out)
        if code != 'succeeded' or after != build:
            raise ValueError('OBS build changed during artifact collection')
        receipt['installation'] = install(out, target, source, build, identity)
        receipt.update(build_verified=True, state='verified')
        save()
    finally:
        if not (out / 'build.log').exists():
            try:
                save_response(obs, target.build + '/_log', out, 'build.log')
            except ValueError as error:
                receipt['build_log_error'] = str(error)
                save()
