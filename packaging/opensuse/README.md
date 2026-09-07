# Experimental package recipe for openSUSE Tumbleweed

This recipe targets x86_64 in an OBS home project. It builds the newest gswitch release tag reachable from the default branch, currently v0.8.0. The package is experimental.

## Prepare the sources

You need an OBS account and a home project with an openSUSE Tumbleweed build target. Install the source preparation tools:

```sh
sudo zypper install osc git go obs-service-obs_scm obs-service-tar obs-service-set_version obs-service-recompress obs-service-go_modules
```

Configure `osc` for your account. Create a regular OBS-managed package, not an SCM-synchronized package. Replace `YOUR_LOGIN` with the OBS project owner's login:

```sh
osc meta pkg -e home:YOUR_LOGIN gswitch
```

The command opens the package XML in an editor and creates the package if it does not exist. Keep the generated package metadata, but do not add a `<scmsync>` element or a Git repository URL. The `_service` file below already fetches the upstream Git sources. If the package metadata currently contains `<scmsync>`, remove that element before continuing; otherwise `osc commit` will skip the package as SCM-managed.

Check out the empty package:

```sh
osc checkout home:YOUR_LOGIN gswitch
cd home:YOUR_LOGIN/gswitch
```

Copy `_service`, `gswitch.spec`, and `gswitch.changes` from this repository folder into the working copy, then run:

```sh
osc service manualrun
osc addremove
```

The `obs_scm` service finds the newest reachable tag matching `v*`, checks out that exact tag, and records its version in OBS metadata. `set_version` reads that metadata and updates the spec, `tar` and `recompress` create the versioned source archive, and `go_modules` reads the generated `gswitch-*.obscpio` rather than guessing among tar archives. The local `third_party/clipboard` replacement remains in the sources and vendor directory. Source preparation needs network access. RPM compilation and Go tests use vendor without network access.

## Submit to your OBS project

Add the recipe and generated service files:

```sh
osc addremove
osc status
osc diff
```

Review the changes before running `osc commit`. Do not add the `gswitch/` Git cache directory left by the service. After submission, check the server build result. To investigate a failure, provide the full build log, OBS package URL, and source revision.

The `manual` mode does not start by itself when a release appears. For an update, verify that the new public release tag is reachable from the default branch and add a `.changes` entry. Remove the previous generated outputs before every run so wildcard inputs cannot see more than one release:

```sh
rm -f gswitch-*.obscpio gswitch-*.tar.gz gswitch.obsinfo vendor.tar.gz
osc service manualrun
osc addremove
osc status
osc diff
```

No version or revision edit is needed: the services select the newest matching tag, derive the version from OBS metadata, update the spec, and regenerate the source and vendor archives. The checked-out tag, rather than the development branch HEAD, remains the reproducible source boundary.

## Verification status

The local OBS services ran through the installed osc dispatcher in a Tumbleweed container. The RPM built without network access; Go tests and desktop file validation passed. Installation and binary dependencies were checked. A modified configuration survived reinstallation of the same version and remained as `.rpmsave` after removal.

The authenticated `osc service manualrun` CLI has not been verified. A separate server recipe has built successfully in `home:arumata/gswitch-automation-test`; its signed RPM passed clean-container installation, reinstallation, configuration preservation, and removal checks. A Tumbleweed user also reported that the tray and correction work with their OBS package. These are separate checks: we have not tested the graphical session, udev ACLs, or polkit dialog ourselves. Official Tumbleweed support is not yet claimed.

After a successful build, check the user service and tray in your graphical session. Test word correction and configuration persistence after logging in again. Run the daemon as the graphical-session user, never as root.

## Packaging findings

rpmlint reports `polkit-untracked-privilege`: the action `com.github.arumata.gswitch.write-config` is absent from the polkit-default-privs profiles. The policy requires administrator authentication and permits temporary retention of that authorization for an active session (`auth_admin_keep`). No filter has been added for this finding.

openSUSE deliberately assigns low badness to these findings in home/devel projects. A first home-project build does not need to wait for an audit. Factory inclusion requires a security team audit and whitelisting of the action. Always check the full build log for your project.

The remaining rpmlint warnings concern PIE for both binaries, local source archive names, and identical Go module license texts. The spec retains lifecycle hooks from the published release, including migration of the old root service. Inclusion in the official openSUSE repository will require a separate packaging review.

## Documentation

OBS: https://openbuildservice.org/help/manuals/obs-user-guide/art-obs-bg

Creating package metadata with osc: https://openbuildservice.org/help/manuals/obs-user-guide/cha-obs-osc

SCM synchronization is a separate workflow: https://openbuildservice.org/help/manuals/obs-user-guide/cha-obs-scm-bridge

Go dependencies: https://github.com/openSUSE/obs-service-go_modules

Home/devel rules: https://en.opensuse.org/openSUSE:Package_security_guidelines#Rpmlint_whitelisting_errors_in_home_and_devel_Projects

## Release automation

The manual `_service` remains available. `_service.release` is the server template for a full release commit SHA and a literal version. The automation uses the author's `arumata` account and the existing `home:arumata` project. Its two allowed targets are `test` (`gswitch-automation-test`) and `main` (`gswitch`). The helper never creates packages, changes publishing settings, or writes to another project.

With Python 3 and the GitHub CLI, prepare the recipe without accessing OBS:

```sh
python3 scripts/obs_release.py --tag v0.8.0 --target test --mode prepare \
  --output builds/obs-prepare
```

Supply the current stable release tag if v0.8.0 is no longer latest. The helper resolves lightweight or annotated tags to a commit. An optional `--commit FULL_SHA` also checks the expected commit, as used by the release workflow. Drafts, prereleases, older releases, moved tags, and non-commit targets are rejected. Version selection does not depend on a development branch or the newest reachable tag.

Use a fresh output directory for each attempt. `receipt.json` records the mode, target, release identity, completed stages, errors, and hashes. The modes are:

| Mode | OBS writes | Result |
| --- | --- | --- |
| `prepare` | None; no OBS access | Generate the exact `_service` locally |
| `check-access` | None | Check authenticated reads of the expected regular OBS package; does not prove write permission |
| `verify` | None | Verify the existing exact recipe, completed build, downloaded artifacts and signed RPM installation |
| `update` | Update `_service` when different | Commit the exact recipe, wait for source services, then perform the same full verification |

OBS access uses `OBS_USERNAME=arumata` and `OBS_PASSWORD`, stored under those same names in GitHub Actions secrets. The credentials have the account's permissions; the helper's target restriction does not reduce server-side account privileges. A package-scoped service token can trigger source services but cannot replace source-write credentials. Do not put passwords in commands, repository files, or logs.

The **OBS** workflow supports manual dispatch with a tag, target and mode. Start with `target=test` and `mode=check-access`; use `update` only after the read-only check and approval to update that package. The manual workflow uses its checked-out implementation and downloads the application source from the selected published release. It does not create tags or GitHub Releases, so an existing release can be checked without republishing it. Only `update` with `source_write_performed=true` establishes that a source write actually happened. An identical recipe is a no-op and reports that honestly.

The Release workflow calls the same OBS workflow for `target=main` after the existing release job and package checks succeed, only when `OBS_ENABLED` equals `true`. Keep this variable unset during setup. The helper and reusable workflow must be present in the released commit. Runs for each target are serialized without cancellation. The two modes `verify` and `update` need Docker on the runner; verification uses a pinned disposable Tumbleweed image.

A successful full check requires all of the following:

- Source services finish with the expected commit and version in `gswitch.obsinfo` and exactly the expected source files.
- The current successful RPM build and latest build-history entry match the verified source `srcmd5` and release version.
- All source files and binary entries are downloaded, with source sizes/digests checked and SHA256 hashes recorded. Both RPM names match the completed build; source/build state is checked again after downloading.
- The downloaded project signing key matches the independently verified key pinned in the helper. Key rotation requires a separate fingerprint review and pin update.
- The RPM signature, package name, architecture, version and `DISTURL` identify the expected signed package and OBS source revision.
- The shared `scripts/obs_verify_rpm.sh` installs that RPM, checks its files/dependencies and installed binary hashes, then verifies reinstallation and removal with configuration preservation. The local `opensuse_package.py --phase verify` uses the same script.

`build_verified=true` is set only after these checks. Container verification does not establish graphical-session behavior, udev ACLs, or cross-version upgrades. The complete rpmlint output is retained; the existing exact `polkit-untracked-privilege` finding remains pending distribution review and is not called a clean lint result. Unexpected rpmlint errors fail the job.

Receipts, logs, sources and binaries are uploaded as workflow artifacts, including partial results on failure. Network-ambiguous writes are never retried automatically. Inspect OBS state before retrying a failed run; use the manual OBS workflow or **Re-run failed jobs** without repeating the release job. A failed source-service run with the correct recipe needs diagnosis and an explicit OBS service trigger before retrying verification. Do not edit the same OBS package concurrently from another client: source-file PUT is not compare-and-swap. The spec and changes file are maintained separately when packaging needs change.

Activation and publishing are separate. Full authenticated CI execution, the main package, and installation from a published repository still need acceptance before enabling automatic release updates. Once published, this is the author's OBS repository for Tumbleweed; these RPMs are not uploaded to GitHub Releases by this workflow.
