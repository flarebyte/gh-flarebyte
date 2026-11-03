#!/usr/bin/env python3
"""Read-only checks for this repository's manually triggered GitHub release."""

import argparse
import hashlib
from http.client import HTTPSConnection, HTTPException
import json
import os
from pathlib import Path
import re
import ssl
import subprocess
import sys
from urllib.parse import quote


class PreflightError(Exception):
    pass


def literal(block, field):
    match = re.search(r'\b' + re.escape(field) + r'\s*:\s*("(?:[^"\\]|\\.)*")', block)
    if not match:
        raise PreflightError(f"Config requires a literal string for {field}")
    value = json.loads(match[1])
    if not value or any(ord(char) < 32 for char in value):
        raise PreflightError(f"Invalid config value for {field}")
    return value


def config_block(source, name):
    match = re.search(r'(?m)^' + re.escape(name) + r'\s*:\s*\{([^{}]*)\}', source)
    if not match:
        raise PreflightError(f"Config requires a literal {name} block")
    return match[1]


def release_metadata(root, expected_version=None):
    """Use the same literal CUE release fields and version syntax as the CLI."""
    source = (root / '.gh-flarebyte.cue').read_text()
    release = config_block(source, 'release')
    build = config_block(source, 'build')
    version_path = root / literal(release, 'versionSource')
    text = version_path.read_text()
    if version_path.suffix == '.json':
        version = json.loads(text).get('version')
    elif version_path.suffix in ('.yaml', '.yml'):
        # The existing CLI accepts an indented version scalar, including tags.version
        # in main.project.yaml. Reject ambiguity rather than guessing between values.
        matches = re.findall(r'(?m)^\s*version:\s*([^\n]+)', text)
        if len(matches) != 1:
            raise PreflightError('Version source must contain exactly one version scalar')
        version = matches[0].strip().strip('"\'')
    else:
        raise PreflightError('This Go release workflow requires a YAML or JSON version source')
    if not isinstance(version, str) or not re.fullmatch(
        r'[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?', version
    ):
        raise PreflightError(f'Invalid release version: {version!r}')
    if expected_version is not None and version != expected_version:
        raise PreflightError(f'Requested version {expected_version!r} differs from configured {version!r}')
    tag = literal(release, 'tagPrefix') + version
    subprocess.run(['git', 'check-ref-format', 'refs/tags/' + tag], check=True, capture_output=True)
    if literal(release, 'notesMode') != 'generate-notes':
        raise PreflightError('Manual release workflow currently requires notesMode: "generate-notes"')
    for field in ('includeArtifacts', 'includeChecksums'):
        if re.search(r'\b' + field + r'\s*:\s*false\b', release):
            raise PreflightError(f'Manual release workflow requires release.{field}: true')
    artifact_dir = literal(release, 'artifactDir')
    path = Path(artifact_dir)
    if path.is_absolute() or '..' in path.parts or path == Path('.'):
        raise PreflightError('release.artifactDir must be a repository-relative subdirectory')
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    if not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise PreflightError('Expected a full Git commit SHA')
    return {
        'version': version,
        'tag': tag,
        'commit': commit,
        'artifact_dir': artifact_dir,
        'checksum_name': Path(literal(build, 'checksumFile')).name,
    }


class GitHub:
    def __init__(self):
        # This repository releases on GitHub.com. Never send its token to a
        # configurable host, and do not follow redirects to another destination.
        if os.environ.get('GITHUB_API_URL', 'https://api.github.com').rstrip('/') != 'https://api.github.com':
            raise PreflightError('Release preflight requires https://api.github.com')
        self.token = os.environ.get('GH_TOKEN') or os.environ.get('GITHUB_TOKEN')

    def get(self, endpoint, missing_ok=False):
        headers = {'Accept': 'application/vnd.github+json', 'User-Agent': 'gh-flarebyte-release-preflight'}
        if self.token:
            headers['Authorization'] = 'Bearer ' + self.token
        if not endpoint.startswith('/repos/') or any(ord(char) < 32 for char in endpoint):
            raise PreflightError('Invalid GitHub API endpoint')
        # Explicit certificate-chain and hostname verification on Python 3.11+.
        context = ssl.create_default_context()
        # Audited: fixed host, verified TLS context, no redirects; covered by tests.
        # nosemgrep: python.lang.security.audit.httpsconnection-detected.httpsconnection-detected
        connection = HTTPSConnection('api.github.com', timeout=30, context=context)
        try:
            connection.request('GET', endpoint, headers=headers)
            with connection.getresponse() as response:
                if response.status == 404 and missing_ok:
                    return None
                if response.status != 200:
                    raise PreflightError(f'GitHub check failed with HTTP {response.status} for {endpoint}')
                return json.load(response)
        except (OSError, HTTPException, ValueError) as error:
            raise PreflightError(f'GitHub check failed for {endpoint}: {error}') from error
        finally:
            connection.close()


def check_available(api, repository, tag):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository):
        raise PreflightError('Repository must be owner/name')
    base = '/repos/' + repository
    # A missing/inaccessible repository must not look like an unused release version.
    repo = api.get(base)
    if not isinstance(repo, dict) or str(repo.get('full_name', '')).lower() != repository.lower():
        raise PreflightError('GitHub returned an unexpected repository')
    if api.get(base + '/git/ref/tags/' + quote(tag, safe=''), missing_ok=True) is not None:
        raise PreflightError(f'Tag {tag} already exists; bump the version before releasing')
    page = 1
    while True:
        releases = api.get(base + f'/releases?per_page=100&page={page}')
        if not isinstance(releases, list):
            raise PreflightError('GitHub returned an invalid release list')
        for release in releases:
            if not isinstance(release, dict) or not isinstance(release.get('tag_name'), str):
                raise PreflightError('GitHub returned an invalid release record')
            if release['tag_name'] == tag:
                raise PreflightError(f'Release {tag} already exists (published or draft)')
        if len(releases) < 100:
            return
        page += 1


def check_artifacts(root, metadata):
    directory = root / metadata['artifact_dir']
    checksum_name = metadata['checksum_name']
    listed = set()
    basenames = {checksum_name}
    for line in (directory / checksum_name).read_text().splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  (.+)', line)
        if not match:
            raise PreflightError('Invalid checksum manifest entry')
        digest, name = match.groups()
        relative = Path(name)
        if relative.is_absolute() or '..' in relative.parts or name in listed:
            raise PreflightError(f'Unsafe or repeated artifact path: {name}')
        if relative.name in basenames:
            raise PreflightError(f'Duplicate release asset filename: {relative.name}')
        basenames.add(relative.name)
        artifact = directory / relative
        if any(path.is_symlink() for path in [directory, artifact, *artifact.parents]):
            raise PreflightError(f'Symlink in artifact path: {name}')
        with artifact.open('rb') as stream:
            actual = hashlib.file_digest(stream, 'sha256').hexdigest()
        if actual != digest:
            raise PreflightError(f'Checksum mismatch for {name}')
        listed.add(name)
    if not listed:
        raise PreflightError('No release artifacts in checksum manifest')
    actual = {path.relative_to(directory).as_posix() for path in directory.rglob('*') if path.is_file()}
    if actual != listed | {checksum_name}:
        raise PreflightError('Release directory contains files outside the checksum manifest')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', default=os.environ.get('GITHUB_REPOSITORY'), required=not os.environ.get('GITHUB_REPOSITORY'))
    parser.add_argument('--expected-version')
    parser.add_argument('--expected-commit')
    parser.add_argument('--check-artifacts', action='store_true')
    parser.add_argument('--github-output', type=Path)
    args = parser.parse_args()
    root = Path.cwd()
    try:
        metadata = release_metadata(root, args.expected_version)
        if args.expected_commit and args.expected_commit != metadata['commit']:
            raise PreflightError('Checkout differs from the commit that passed release verification')
        check_available(GitHub(), args.repository, metadata['tag'])
        if args.check_artifacts:
            check_artifacts(root, metadata)
        if args.github_output:
            with args.github_output.open('a') as stream:
                for key, value in metadata.items():
                    stream.write(f'{key}={value}\n')
        print(json.dumps(metadata, indent=2))
        return 0
    except (PreflightError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f'Release preflight failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
