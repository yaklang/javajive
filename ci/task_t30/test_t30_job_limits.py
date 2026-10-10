"""Container process caps must apply per job, not across a shared host UID."""
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from tools.sandbox_worker.backends.docker import run_docker
from tools.sandbox_worker.job import UntrustedJob
from tools.sandbox_worker.policy import Limits


class DockerJobLimits(unittest.TestCase):
    def test_process_cap_is_scoped_to_container(self):
        for pids in (0, 16, 512):
            with self.subTest(pids=pids), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                inputs, artifacts = root / 'inputs', root / 'artifacts'
                inputs.mkdir(); artifacts.mkdir()
                limits = Limits(pids=pids, memory_bytes=32 * 1024**2)
                with patch('tools.sandbox_worker.backends.docker.image_id', return_value='test-image'), \
                     patch('tools.sandbox_worker.backends.docker.subprocess.Popen', side_effect=OSError('launch stopped')) as launch, \
                     patch('tools.sandbox_worker.backends.docker.cleanup_job_containers', return_value={'verified': True}):
                    result = run_docker(engine='docker', engine_path='/docker',
                        job=UntrustedJob(argv=['true'], limits=limits), inputs=inputs,
                        artifacts=artifacts, job_id='limits-proof', image='test-image')
                argv = launch.call_args.args[0]
                self.assertEqual(result.resource_limits['pids_enforced'], pids > 0)
                self.assertEqual([a for a in argv if a.startswith('--pids-limit=')],
                                 [f'--pids-limit={pids}'] if pids else [])
                self.assertFalse(any(a.startswith('--ulimit=nproc=') for a in argv))
                self.assertIn('--memory=33554432', argv)
                self.assertIn('--network=none', argv)
                self.assertIn('--read-only', argv)
                self.assertTrue(result.extra['leftover_verified'])
