"""Feature-path receipt only. Does not replace any performance/quality gate."""
import argparse
import json
from pathlib import Path

def single_send_allowed(manifest):
    config = manifest.get('config') or {}
    return (config.get('profile_screen') is True and config.get('fec') == 'off'
            and config.get('fec_parity') == 0)


def validate_counters(last, allow_single_send=False):
    errors, observations = [], {}
    for direction in ('receive', 'send'):
        calls, messages, multi = (last[direction + '_' + field] for field in ('calls', 'messages', 'multi'))
        if messages <= 0 or calls <= 0 or multi < 0 or last[direction + '_fallbacks'] != 0:
            errors.append(direction + ': no native traffic or fallback occurred')
        elif multi == 0 and not (direction == 'send' and allow_single_send):
            errors.append(direction + ': native multi-message path not exercised')
        observations[direction] = ('MULTI_MESSAGE_OBSERVED' if multi > 0 else
                                   'SINGLE_MESSAGE_ONLY_MULTI_NOT_EXERCISED')
    return errors, observations


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--artifact-dir', required=True)
    args = p.parse_args()
    root = Path(args.artifact_dir)
    manifest_path = root / 'manifest.json'
    allow_single = single_send_allowed(json.loads(manifest_path.read_text())) if manifest_path.exists() else False
    receipt = {'scope': 'feature-path counters, not resource attribution', 'endpoints': {},
               'allow_single_send': allow_single, 'observations': {}, 'errors': []}
    for endpoint in ('client', 'server'):
        rows = [json.loads(line) for line in (root / f'{endpoint}-diag.jsonl').read_text().splitlines() if line.strip()]
        products = [row.get('product', {}) for row in rows]
        counters = [product['raw_io'] for product in products if product.get('raw_io', {}).get('enabled')]
        if not counters:
            receipt['errors'].append(endpoint + ': missing enabled raw IO diagnostics')
            continue
        last = counters[-1]
        errors, observations = validate_counters(last, allow_single)
        receipt['errors'].extend(endpoint + '/' + error for error in errors)
        receipt['observations'][endpoint] = observations
        receipt['endpoints'][endpoint] = last
    (root / 'raw-io-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print('WBD_RAW_IO_RECEIPT ' + json.dumps(receipt, sort_keys=True))
    raise SystemExit(1 if receipt['errors'] else 0)


if __name__ == '__main__':
    main()
