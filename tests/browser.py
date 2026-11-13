"""Real-browser keyboard regression suite; no frontend build or runtime deps.

Run: python -m pip install -r tests/requirements.txt
     python -m playwright install chromium
     python tests/browser.py
Optional: HDECK_TEST_BINARY=/path/to/hdeck HDECK_BROWSER_PATH=/path/to/chromium
Fixtures, config, responses, browser state and the server all live in temp dirs.
"""
import datetime
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.request

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def seed(root):
    now = datetime.datetime.now(datetime.timezone.utc)
    rows = [
        ('zeta', 'newest', 'Keyboard navigation review', 1),
        ('alpha', 'middle', 'Migration readiness audit', 3),
        ('zeta', 'older', 'Renderer improvements', 0),
        ('alpha', 'oldest', 'Choose the rollout strategy', 2),
    ]
    for i, (project, run, title, asks) in enumerate(rows):
        directory = root / 'reports' / project / run
        directory.mkdir(parents=True)
        blocks = [{'type': 'prose', 'title': 'Summary', 'markdown': 'Review the **keyboard** workflow.\n\nUse j/k and Enter.'}]
        for n in range(asks):
            blocks.append({'type': 'ask', 'id': f'ask-{n}', 'title': f'Review choice {n+1}',
                           'prompt': 'Which approach?', 'mode': 'choice', 'options': ['Incremental rollout', 'Full rollout']})
        report = {'schema': 'harness-deck/report@1', 'id': run, 'project': project,
                  'harness': 'codex', 'title': title, 'status': 'awaiting-review', 'kind': 'review',
                  'created': (now - datetime.timedelta(hours=i)).isoformat(), 'blocks': blocks}
        (directory / 'report.json').write_text(json.dumps(report))
        docs = root / 'projects' / project / '.docs' / 'ai'
        docs.mkdir(parents=True, exist_ok=True)
        (docs / 'current-state.md').write_text('# Current state\n\nReady for review.\n')
        (docs / 'roadmap.md').write_text('# Roadmap\n\n- Preserve keyboard workflows\n')


class BrowserTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build_dir = tempfile.TemporaryDirectory(prefix='hdeck-build-')
        cls.binary = os.environ.get('HDECK_TEST_BINARY', str(Path(cls.build_dir.name) / 'hdeck'))
        if not os.environ.get('HDECK_TEST_BINARY'):
            subprocess.run(['go', 'build', '-o', cls.binary, './cmd/harness-deck'], cwd=ROOT, check=True)
        cls.playwright = sync_playwright().start()
        options = {}
        if os.environ.get('HDECK_BROWSER_PATH'):
            options['executable_path'] = os.environ['HDECK_BROWSER_PATH']
        cls.browser = cls.playwright.chromium.launch(**options)

    @classmethod
    def tearDownClass(cls):
        cls.browser.close()
        cls.playwright.stop()
        cls.build_dir.cleanup()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='hdeck-browser-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        seed(self.root)
        for manifest in (self.root / 'reports').glob('*/*/report.json'):
            result = subprocess.run([self.binary, 'validate', str(manifest)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        config = {'central_dir': str(self.root / 'reports'), 'scan_roots': [str(self.root / 'projects')],
                  'bind': '127.0.0.1', 'port': port}
        path = self.root / 'config.json'
        path.write_text(json.dumps(config))
        self.server = subprocess.Popen([self.binary, 'serve'], env={**os.environ, 'HARNESS_DECK_CONFIG': str(path)},
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(self.stop_server)
        self.url = f'http://127.0.0.1:{port}'
        for _ in range(150):
            try:
                with urllib.request.urlopen(self.url + '/api/reports', timeout=1):
                    break
            except OSError:
                if self.server.poll() is not None:
                    self.fail('Fixture server exited before startup')
                time.sleep(.05)
        else:
            self.fail('Fixture server never became ready')
        self.context = self.browser.new_context(viewport={'width': 1440, 'height': 960}, color_scheme='dark',
                                               reduced_motion='reduce', service_workers='block')
        self.addCleanup(self.context.close)
        # A local/offline app must also work without Google Fonts.
        self.context.route('https://fonts.googleapis.com/**', lambda route: route.abort())
        self.context.route('https://fonts.gstatic.com/**', lambda route: route.abort())
        self.page = self.context.new_page()
        self.errors = []
        self.page.on('pageerror', lambda error: self.errors.append(str(error)))
        self.page.goto(self.url)
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)

    def tearDown(self):
        self.assertEqual(self.errors, [], 'Uncaught browser errors')

    def stop_server(self):
        self.server.terminate()
        try:
            self.server.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.server.kill()
            self.server.wait()

    def keys(self, *keys):
        for key in keys:
            self.page.keyboard.press(key)

    def normal(self):
        self.page.evaluate('document.activeElement.blur()')
        expect(self.page.locator('#status')).to_have_attribute('data-vim-mode', 'NORMAL')

    def focused(self):
        return self.page.locator('#view-inbox .inbox-item.focused .title').inner_text()

    def report(self, project='alpha', run='middle'):
        self.page.goto(self.url + f'/r/{project}/{run}')
        expect(self.page.locator('h1')).to_be_visible()
        expect(self.page.locator('.block-fallback')).to_have_count(0)

    def rewrite_blocks(self, blocks):
        path = self.root / 'reports' / 'alpha' / 'middle' / 'report.json'
        report = json.loads(path.read_text())
        report['blocks'] = blocks
        path.write_text(json.dumps(report))
        self.report()

    def test_identity_and_navigation_chords(self):
        expect(self.page).to_have_title('(6) harness-deck')
        self.keys('g', 'p')
        expect(self.page.locator('#view-projects')).to_be_visible()
        self.keys('g', 'l')
        expect(self.page.locator('#view-activity .activity-row')).to_have_count(4)
        self.keys(']', '[', '{', '}')  # no section targets on the activity view
        self.keys('g', 'i')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)
        self.keys(':')
        self.page.locator('#vim-prompt input').fill('projects')
        self.keys('Enter')
        expect(self.page.locator('#view-projects')).to_be_visible()

    def test_cursor_follows_every_sort_and_opens_visible_report(self):
        for _ in range(3):
            self.page.locator('.sort-toggle').click()
            self.normal()
            titles = self.page.locator('#view-inbox .inbox-item .title').all_text_contents()
            self.keys('G')
            self.assertEqual(self.focused(), titles[-1])
            self.keys('k')
            self.assertEqual(self.focused(), titles[-2])
            self.keys('j')
            self.assertEqual(self.focused(), titles[-1])
        expected = self.page.locator('#view-inbox .inbox-item.focused').get_attribute('data-url')
        self.keys('Enter')
        expect(self.page).to_have_url(self.url + expected)

    def test_cursor_survives_reload_and_live_update(self):
        self.keys('j')
        selected = self.focused()
        self.page.reload()
        expect(self.page.locator('.inbox-item.focused .title')).to_have_text(selected)
        # A server-side change triggers the real SSE watcher, not a mocked reload.
        path = self.root / 'reports' / 'zeta' / 'older' / 'report.json'
        report = json.loads(path.read_text())
        report['title'] = 'Live update received'
        path.write_text(json.dumps(report))
        expect(self.page.locator('#view-inbox .title', has_text='Live update received')).to_be_visible(timeout=10000)
        self.assertEqual(self.focused(), selected)

    def test_native_control_focus_survives_refresh(self):
        button = self.page.locator('.sort-toggle')
        button.focus()
        self.page.evaluate('HarnessDeck.reload()')
        expect(button).to_be_focused()
        self.keys('Enter')
        expect(button).to_have_text('sort: asks')
        expect(button).to_be_focused()
        link = self.page.locator('#view-inbox .inbox-item .title').first
        link.focus()
        self.page.evaluate('HarnessDeck.reload()')
        expect(link).to_be_focused()

    def test_native_link_activation_does_not_open_cursor(self):
        link = self.page.locator('#view-inbox .inbox-item .title').nth(2)
        link.focus()
        self.keys('Enter')
        expect(self.page).to_have_url(self.url + '/r/zeta/older')

    def test_archive_restore_and_close(self):
        self.keys('a')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(3)
        self.keys('g', 'a')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(1)
        self.keys('a')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(0)
        self.keys('g', 'i', 'j', 'j', 'x')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(3)

    def test_delete_requires_confirmation(self):
        self.page.once('dialog', lambda dialog: dialog.dismiss())
        self.keys('d', 'd')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)
        self.page.once('dialog', lambda dialog: dialog.accept())
        self.keys('d', 'd')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(3)

    def test_pins_tree_and_digit_jump(self):
        self.keys('p')
        expect(self.page.locator('.pinned-tree .row')).to_have_count(1)
        self.keys('Space', 'e')
        expect(self.page.locator('.tree-active')).to_have_count(1)
        self.keys('j', 'k')
        expect(self.page.locator('.tree-active')).to_be_focused()
        self.keys('Escape', '2')
        expect(self.page).to_have_url(self.url + '/r/zeta/newest')
        self.keys('g', 'x')
        expect(self.page.locator('#hd-pin-btn')).to_have_attribute('data-pinned', 'false')
        # g x unpins; it must never fall through to the report's x (close).
        self.assertEqual(json.loads((self.root / 'reports/zeta/newest/report.json').read_text())['status'], 'awaiting-review')
        self.keys('q')
        expect(self.page).to_have_url(self.url + '/')

    def test_overlays_trap_focus_restore_and_block_actions(self):
        trigger = self.page.get_by_role('button', name='Settings', exact=True)
        trigger.click()
        modal = self.page.get_by_role('dialog', name='Settings', exact=True)
        expect(modal).to_be_visible()
        close = modal.get_by_role('button', name='Close settings')
        expect(close).to_be_focused()
        self.keys('x', 'a', 'p', 'g', 'p')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)
        expect(self.page.locator('#view-projects')).not_to_be_visible()
        self.keys('Shift+Tab')
        self.assertTrue(modal.evaluate('(node) => node.contains(document.activeElement)'))
        self.keys('Tab')
        expect(close).to_be_focused()
        self.keys('Escape')
        expect(modal).not_to_be_visible()
        expect(trigger).to_be_focused()
        self.normal()
        self.keys('?')
        expect(self.page.get_by_role('dialog', name='Keyboard shortcuts')).to_be_visible()
        self.keys('x', 'a', 'Escape')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)

    def test_select_and_inputs_do_not_trigger_global_shortcuts(self):
        self.keys('Space', 's')
        select = self.page.get_by_label('Destination type')
        select.focus()
        self.keys('x', 'a', 'p', 'Space')
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)
        self.page.get_by_label('Destination name').fill('xap jk')
        expect(self.page.get_by_label('Destination name')).to_have_value('xap jk')
        self.keys('Escape')
        expect(self.page.get_by_role('dialog')).not_to_be_visible()

    def test_theme_cycle_and_persistence(self):
        self.keys('Space', 't')
        expect(self.page.locator('html')).to_have_attribute('data-theme', 'dark')
        self.keys('Space', 't')
        expect(self.page.locator('html')).to_have_attribute('data-theme', 'light')
        self.page.reload()
        expect(self.page.locator('html')).to_have_attribute('data-theme', 'light')
        self.keys('Space', 't')
        self.assertIsNone(self.page.locator('html').get_attribute('data-theme'))

    def test_search_keyboard_and_native_button(self):
        trigger = self.page.get_by_role('button', name='Search reports', exact=True)
        trigger.click()
        field = self.page.get_by_role('textbox', name='Search reports')
        expect(field).to_be_focused()
        field.fill('Migration')
        expect(self.page.locator('.search-row')).to_have_count(1)
        self.page.once('dialog', lambda dialog: dialog.accept('Migrations'))
        self.keys('Control+s')
        expect(self.page.locator('.search-status')).to_have_text('saved ✓')
        self.keys('Escape')
        expect(trigger).to_be_focused()
        saved = self.page.get_by_role('button', name='Migrations', exact=True)
        saved.focus()
        self.keys('Enter')
        expect(field).to_have_value('Migration')
        self.keys('Escape')
        expect(saved).to_be_focused()
        self.normal()
        self.keys('Control+k')
        field.fill('Migration')
        expect(self.page.locator('.search-row')).to_have_count(1)
        self.keys('Enter')
        expect(self.page).to_have_url(self.url + '/r/alpha/middle')

    def test_search_failure_does_not_retry_forever(self):
        requests = []
        def fail(route):
            requests.append(route.request.url)
            route.fulfill(status=503, body='unavailable')
        self.page.route('**/api/search?q=*', fail)
        self.keys('Control+k')
        self.page.get_by_role('textbox', name='Search reports').fill('Migration')
        expect(self.page.locator('.search-status')).to_have_text('search failed')
        self.page.wait_for_timeout(450)
        self.assertEqual(len(requests), 1)
        self.keys('Escape')
        expect(self.page.locator('#search-palette')).not_to_be_visible()

    def test_search_discards_stale_response_after_clear(self):
        self.page.evaluate('''() => {
            const original = window.fetch;
            window.fetch = (url, options) => String(url).includes('/api/search?q=slow')
                ? new Promise(resolve => setTimeout(() => resolve(new Response(JSON.stringify({matches: [{project:'stale', run:'stale', title:'Stale result'}]}))), 400))
                : original(url, options);
        }''')
        self.keys('Control+k')
        field = self.page.get_by_role('textbox', name='Search reports')
        field.fill('slow')
        self.page.wait_for_timeout(180)
        field.fill('')
        self.page.wait_for_timeout(500)
        expect(self.page.locator('.search-row')).to_have_count(0)
        expect(self.page.locator('.search-status')).to_have_text('')

    def test_report_choice_navigation_and_submission(self):
        self.report()
        expect(self.page.locator('.ask-focused')).to_have_id('sec-2')
        self.keys('Tab')
        expect(self.page.locator('.ask-focused')).to_have_id('sec-3')
        self.keys('Shift+Tab', 'l', 'Enter')
        expect(self.page.locator('#sec-2 .ask-answered')).to_be_visible()
        responses = json.loads((self.root / 'reports/alpha/middle/responses.json').read_text())
        self.assertIn('Full rollout', json.dumps(responses))
        expect(self.page.locator('.ask-focused')).to_have_id('sec-3')
        self.keys('1')
        expect(self.page.locator('#sec-3 .ask-answered')).to_be_visible()

    def test_report_text_input_escape_and_submit(self):
        self.rewrite_blocks([{'type': 'ask', 'id': 'text', 'title': 'Name', 'prompt': 'Enter a name', 'mode': 'text'}])
        self.keys('i')
        field = self.page.locator('.ask-text-input')
        expect(field).to_be_focused()
        field.fill('x a p j k')
        self.keys('Escape')
        expect(self.page.locator('#status')).to_have_attribute('data-vim-mode', 'NORMAL')
        self.keys('l', 'Enter')
        expect(self.page.locator('.ask-answered')).to_contain_text('x a p j k')

    def test_report_help_blocks_actions(self):
        self.report('zeta', 'older')
        self.keys('?')
        expect(self.page.get_by_role('dialog', name='Keyboard shortcuts')).to_be_visible()
        self.keys('x', 'a', 'p', '1', 'Tab')
        expect(self.page).to_have_url(self.url + '/r/zeta/older')
        self.keys('Escape')
        expect(self.page.get_by_role('dialog')).not_to_be_visible()

    def test_report_g_archive_is_navigation_not_mutation(self):
        self.report('zeta', 'older')
        self.keys('g', 'a')
        expect(self.page).to_have_url(self.url + '/')
        self.assertFalse(json.loads((self.root / 'reports/zeta/older/report.json').read_text()).get('archived', False))

    def test_yesno_approval_decision_and_multi_answers(self):
        cases = [
            ({'type': 'ask', 'id': 'yesno', 'prompt': 'Continue?', 'mode': 'yesno'}, ['y'], 'yes'),
            ({'type': 'approval', 'id': 'approval', 'prompt': 'Approve changes?'}, ['n'], 'changes-requested'),
            ({'type': 'decision', 'id': 'decision', 'prompt': 'Choose a path',
              'a': {'tag': 'A', 'title': 'Incremental'}, 'b': {'tag': 'B', 'title': 'Full'}}, ['2'], 'B'),
            ({'type': 'ask', 'id': 'multi', 'prompt': 'Choose components', 'mode': 'multi',
              'options': ['UI', 'Keyboard', 'Docs']}, ['1', '2', 'Enter'], 'Keyboard'),
        ]
        for block, keys, value in cases:
            with self.subTest(block=block['type']):
                self.rewrite_blocks([block])
                self.keys(*keys)
                expect(self.page.locator('.ask-answered')).to_contain_text(value)

    def test_section_jumps_clear_sticky_header(self):
        self.rewrite_blocks([{'type': 'prose', 'title': f'Section {n}', 'markdown': 'Paragraph.\n\n' * 12} for n in range(6)])
        self.keys('g', 'g', ']')
        expect(self.page.locator('#sec-1')).to_have_class('panel vim-flash')
        self.keys(']')
        expect(self.page.locator('#sec-2')).to_have_class('panel vim-flash')
        self.assertGreaterEqual(self.page.locator('#sec-2').bounding_box()['y'],
                                self.page.locator('.term-chrome').bounding_box()['height'])
        self.keys('[')
        expect(self.page.locator('#sec-1')).to_have_class('panel vim-flash')

    def test_project_disclosures_support_native_keyboard(self):
        self.keys('g', 'p')
        tracked = self.page.locator('.tracked-head')
        tracked.focus()
        self.keys('Enter')
        expect(tracked).to_have_attribute('aria-expanded', 'true')
        expect(tracked).to_be_focused()
        disclosure = self.page.locator('button.proj-sec').first
        disclosure.focus()
        self.keys('Space')
        expect(disclosure).to_have_attribute('aria-expanded', 'false')
        expect(disclosure).to_be_focused()

    def test_project_history_links_use_parent_project(self):
        self.keys('g', 'p')
        links = self.page.locator('#view-projects .hist-head')
        expect(links).to_have_count(4)
        for href in links.evaluate_all('(nodes) => nodes.map(n => n.getAttribute("href"))'):
            self.assertNotIn('undefined', href)
        link = links.first
        url = link.get_attribute('href')
        link.focus()
        self.keys('Enter')
        expect(self.page).to_have_url(self.url + url)
        expect(self.page.locator('h1')).to_be_visible()

    def test_ime_composition_does_not_trigger_actions(self):
        self.page.evaluate("document.body.dispatchEvent(new KeyboardEvent('keydown', {key:'x', isComposing:true, bubbles:true}))")
        expect(self.page.locator('#view-inbox .inbox-item')).to_have_count(4)

    def test_responsive_layout_drawer_and_focus(self):
        for width in [360, 393, 768, 1024, 1440]:
            self.page.set_viewport_size({'width': width, 'height': 900})
            self.assertLessEqual(self.page.evaluate('document.documentElement.scrollWidth'), width)
        self.page.set_viewport_size({'width': 393, 'height': 852})
        sidebar = self.page.locator('.sidebar')
        self.assertTrue(sidebar.evaluate('(node) => node.inert'))
        menu = self.page.get_by_role('button', name='Toggle navigation')
        menu.click()
        expect(menu).to_have_attribute('aria-expanded', 'true')
        expect(sidebar).to_have_attribute('role', 'dialog')
        self.keys('Shift+Tab', 'Tab')
        self.assertTrue(sidebar.evaluate('(node) => node.contains(document.activeElement)'))
        self.keys('Escape')
        expect(menu).to_be_focused()
        expect(menu).to_have_attribute('aria-expanded', 'false')
        self.assertTrue(sidebar.evaluate('(node) => node.inert'))
        menu.click()
        self.page.locator('.drawer-scrim').click(position={'x': 380, 'y': 400})
        expect(menu).to_have_attribute('aria-expanded', 'false')
        self.report()
        self.assertLessEqual(self.page.evaluate('document.documentElement.scrollWidth'), 393)
        self.keys('G')
        self.assertAlmostEqual(self.page.locator('.term-chrome').bounding_box()['y'], 0, delta=1)
        self.page.get_by_role('button', name='Toggle navigation').click()
        expect(sidebar).to_have_attribute('role', 'dialog')
        self.keys('Escape')


if __name__ == '__main__':
    unittest.main(verbosity=2)
