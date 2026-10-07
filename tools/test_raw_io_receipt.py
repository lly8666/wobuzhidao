import unittest

from collect_raw_io_receipt import single_send_allowed, validate_counters


class RawIOReceiptTests(unittest.TestCase):
    def counters(self):
        return dict(receive_calls=2, receive_messages=3, receive_multi=1,
                    receive_fallbacks=0, send_calls=2, send_messages=2,
                    send_multi=0, send_fallbacks=0)

    def test_only_explicit_off_screen_allows_single(self):
        self.assertTrue(single_send_allowed({'config': dict(profile_screen=True, fec='off', fec_parity=0)}))
        for config in ({}, dict(fec='off', fec_parity=0),
                       dict(profile_screen=True, fec='20:4', fec_parity=4),
                       dict(profile_screen=True, fec='off', fec_parity=4)):
            self.assertFalse(single_send_allowed({'config': config}))

    def test_off_does_not_need_coalescing_and_reports_unexercised(self):
        errors, observations = validate_counters(self.counters(), True)
        self.assertEqual(errors, [])
        self.assertEqual(observations['send'], 'SINGLE_MESSAGE_ONLY_MULTI_NOT_EXERCISED')
        self.assertTrue(validate_counters(self.counters())[0])

    def test_off_still_rejects_fallback_empty_send_or_empty_receive(self):
        for key in ('send_fallbacks', 'receive_fallbacks', 'send_calls',
                    'send_messages', 'receive_calls', 'receive_messages', 'receive_multi'):
            counters = self.counters()
            counters[key] = 1 if key.endswith('fallbacks') else 0
            self.assertTrue(validate_counters(counters, True)[0], key)

    def test_original_multi_path_keeps_existing_requirement(self):
        counters = self.counters()
        counters['send_multi'] = 1
        counters['send_messages'] = 3
        self.assertEqual(validate_counters(counters)[0], [])


if __name__ == '__main__':
    unittest.main()
