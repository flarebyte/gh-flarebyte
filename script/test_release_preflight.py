import hashlib
import io
import json
import ssl
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from release_preflight import (
    GitHub,
    PreflightError,
    check_artifacts,
    check_available,
    main,
    release_metadata,
)


class FakeGitHub:
    def __init__(self, tag=None, pages=None):
        self.tag = tag
        self.pages = pages or [[]]
        self.calls = []

    def get(self, endpoint, missing_ok=False):
        self.calls.append(endpoint)
        if '/git/ref/tags/' in endpoint:
            return self.tag
        if '/releases?' in endpoint:
            return self.pages[int(endpoint.rsplit('=', 1)[1]) - 1]
        return {'full_name': 'flarebyte/gh-flarebyte'}


class ReleasePreflightTests(unittest.TestCase):
    @staticmethod
    def response(status=200, body='{}'):
        response = io.StringIO(body)
        response.status = status
        return response

    def test_available_version_is_read_only(self):
        api = FakeGitHub()
        check_available(api, 'flarebyte/gh-flarebyte', 'v1.2.3')
        self.assertEqual(len(api.calls), 3)

    def test_existing_tag_blocks_release_even_without_a_release_record(self):
        with self.assertRaisesRegex(PreflightError, 'Tag .* already exists'):
            check_available(FakeGitHub(tag={'ref': 'refs/tags/v1.2.3'}), 'flarebyte/gh-flarebyte', 'v1.2.3')

    def test_published_and_draft_releases_are_rejected_on_later_pages(self):
        for draft in (False, True):
            api = FakeGitHub(pages=[
                [{'tag_name': 'older'}] * 100,
                [{'tag_name': 'v1.2.3', 'draft': draft}],
            ])
            with self.subTest(draft=draft), self.assertRaisesRegex(PreflightError, 'Release .* already exists'):
                check_available(api, 'flarebyte/gh-flarebyte', 'v1.2.3')

    def test_api_errors_do_not_count_as_available(self):
        api = GitHub()
        for status in (301, 302, 307, 308, 401, 403, 429, 500):
            with self.subTest(status=status), patch('release_preflight.HTTPSConnection') as connection:
                connection.return_value.getresponse.return_value = self.response(status)
                with self.assertRaisesRegex(PreflightError, f'HTTP {status}'):
                    api.get('/repos/flarebyte/gh-flarebyte/git/ref/tags/v1.2.3', missing_ok=True)
                connection.return_value.request.assert_called_once()
                connection.return_value.close.assert_called_once()
        with patch('release_preflight.HTTPSConnection') as connection:
            connection.return_value.getresponse.side_effect = [self.response(404), self.response(404)]
            with self.assertRaisesRegex(PreflightError, 'HTTP 404'):
                api.get('/repos/flarebyte/gh-flarebyte')
            self.assertIsNone(api.get('/repos/flarebyte/gh-flarebyte/git/ref/tags/v1.2.3', missing_ok=True))

    def test_invalid_json_and_release_records_fail_closed(self):
        with patch('release_preflight.HTTPSConnection') as connection:
            connection.return_value.getresponse.return_value = self.response(body='invalid json')
            with self.assertRaises(PreflightError):
                GitHub().get('/repos/flarebyte/gh-flarebyte')
        for invalid in ({'message': 'error'}, [{}]):
            with self.subTest(invalid=invalid), self.assertRaises(PreflightError):
                check_available(FakeGitHub(pages=[invalid]), 'flarebyte/gh-flarebyte', 'v1.2.3')

    def test_api_destination_is_fixed_and_token_is_only_sent_over_https(self):
        for base in ('file:///etc/passwd', 'http://api.github.com', 'https://evil.example',
                     'https://api.github.com.evil.example', 'https://api.github.com@evil.example',
                     'https://api.github.com/path', 'https://api.github.com?host=evil'):
            with self.subTest(base=base), patch.dict('os.environ', {'GITHUB_API_URL': base}):
                with self.assertRaisesRegex(PreflightError, 'requires https://api.github.com'):
                    GitHub()
        with patch.dict('os.environ', {'GITHUB_API_URL': 'https://api.github.com', 'GH_TOKEN': 'test-token'}):
            api = GitHub()
        with patch('release_preflight.HTTPSConnection') as connection:
            connection.return_value.getresponse.return_value = self.response(body='{"ok": true}')
            self.assertEqual(api.get('/repos/flarebyte/gh-flarebyte'), {'ok': True})
            connection.assert_called_once()
            connection_args, connection_kwargs = connection.call_args
            self.assertEqual(connection_args, ('api.github.com',))
            self.assertEqual(connection_kwargs['timeout'], 30)
            context = connection_kwargs['context']
            self.assertIsInstance(context, ssl.SSLContext)
            self.assertEqual(context.verify_mode, ssl.CERT_REQUIRED)
            self.assertTrue(context.check_hostname)
            self.assertGreater(context.cert_store_stats()['x509_ca'], 0)
            args, kwargs = connection.return_value.request.call_args
            self.assertEqual(args, ('GET', '/repos/flarebyte/gh-flarebyte'))
            self.assertEqual(kwargs['headers']['Authorization'], 'Bearer test-token')
            connection.return_value.close.assert_called_once()
        for endpoint in ('file:///etc/passwd', '//evil.example/path', '/repos/owner/repo\r\nInjected: yes'):
            with self.subTest(endpoint=endpoint), patch('release_preflight.HTTPSConnection') as connection:
                with self.assertRaisesRegex(PreflightError, 'Invalid GitHub API endpoint'):
                    api.get(endpoint)
                connection.assert_not_called()

    def test_network_failures_stop_preflight_and_close_connection(self):
        for error in (TimeoutError('timed out'), ssl.SSLCertVerificationError('untrusted certificate')):
            with self.subTest(error=error), patch('release_preflight.HTTPSConnection') as connection:
                connection.return_value.request.side_effect = error
                with self.assertRaisesRegex(PreflightError, 'GitHub check failed'):
                    GitHub().get('/repos/flarebyte/gh-flarebyte')
                connection.return_value.close.assert_called_once()

    def test_version_source_matches_existing_nested_yaml_and_json(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = '''build: { checksumFile: "build/checksums.txt" }
release: {
 versionSource: "version.yaml"
 tagPrefix: "v"
 artifactDir: "build"
 notesMode: "generate-notes"
}
'''
            (root / '.gh-flarebyte.cue').write_text(config)
            (root / 'version.yaml').write_text('tags:\n  version: 1.2.3\n')
            with patch('release_preflight.subprocess.check_output', return_value='a' * 40):
                metadata = release_metadata(root)
                self.assertEqual(metadata['version'], '1.2.3')
                self.assertEqual(metadata['tag'], 'v1.2.3')
                self.assertEqual(metadata['commit'], 'a' * 40)
                with self.assertRaisesRegex(PreflightError, 'differs from configured'):
                    release_metadata(root, '1.2.4')
                (root / 'version.yaml').write_text('version: 1.2.3\ntags:\n  version: 1.2.4\n')
                with self.assertRaisesRegex(PreflightError, 'exactly one'):
                    release_metadata(root)
                (root / '.gh-flarebyte.cue').write_text(config.replace('version.yaml', 'version.json'))
                (root / 'version.json').write_text(json.dumps({'version': '1.2.3'}))
                self.assertEqual(release_metadata(root)['version'], '1.2.3')

    def test_preflight_resolves_version_without_input_and_exports_it_for_publish(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / 'github-output'
            (root / '.gh-flarebyte.cue').write_text('''build: { checksumFile: "build/checksums.txt" }
release: {
 versionSource: "main.project.yaml"
 tagPrefix: "v"
 artifactDir: "build"
 notesMode: "generate-notes"
}
''')
            (root / 'main.project.yaml').write_text('tags:\n  version: 2.3.4\n')
            with (
                patch('release_preflight.Path.cwd', return_value=root),
                patch('release_preflight.subprocess.check_output', return_value='a' * 40),
                patch('release_preflight.GitHub', return_value=FakeGitHub()),
                patch('sys.stdout', new_callable=io.StringIO) as stdout,
                patch('sys.argv', ['release_preflight.py', '--repository', 'flarebyte/gh-flarebyte', '--github-output', str(output)]),
            ):
                self.assertEqual(main(), 0)
            self.assertEqual(json.loads(stdout.getvalue())['version'], '2.3.4')
            outputs = dict(line.split('=', 1) for line in output.read_text().splitlines())
            self.assertEqual(outputs['version'], '2.3.4')
            self.assertEqual(outputs['tag'], 'v2.3.4')
            self.assertEqual(outputs['commit'], 'a' * 40)

    def test_downloaded_artifacts_must_match_checksums_and_have_unique_names(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            build = root / 'build'
            build.mkdir()
            artifact = build / 'cli'
            artifact.write_bytes(b'compiled CLI')
            digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
            checksum = build / 'checksums.txt'
            checksum.write_text(f'{digest}  cli\n')
            metadata = {'artifact_dir': 'build', 'checksum_name': 'checksums.txt'}
            check_artifacts(root, metadata)
            artifact.write_bytes(b'corrupted transfer')
            with self.assertRaisesRegex(PreflightError, 'Checksum mismatch'):
                check_artifacts(root, metadata)
            artifact.write_bytes(b'compiled CLI')
            (build / 'stale').write_bytes(b'stale artifact')
            with self.assertRaisesRegex(PreflightError, 'outside the checksum manifest'):
                check_artifacts(root, metadata)
            (build / 'stale').unlink()
            (build / 'other').mkdir()
            (build / 'other' / 'cli').write_bytes(b'compiled CLI')
            checksum.write_text(f'{digest}  cli\n{digest}  other/cli\n')
            with self.assertRaisesRegex(PreflightError, 'Duplicate release asset filename'):
                check_artifacts(root, metadata)
            checksum.write_text(f'{digest}  ../outside\n')
            with self.assertRaisesRegex(PreflightError, 'Unsafe or repeated'):
                check_artifacts(root, metadata)


if __name__ == '__main__':
    unittest.main()
