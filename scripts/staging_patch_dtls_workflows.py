from pathlib import Path

ROOT = Path('.github/workflows')
MACRO = 'WOLFSSL_DTLS_WINDOW_WORDS=128'
STAGING = 'agent/fec-zombie-fix-7d859'


def replace_exact(text: str, old: str, new: str, *, count: int = 1, path: Path) -> str:
    got = text.count(old)
    if got != count:
        raise SystemExit(f'{path}: expected {count} occurrences, found {got}: {old!r}')
    return text.replace(old, new, count)


def add_staging_branch(text: str, path: Path) -> str:
    marker = '      - feat/single-flow-reality-faketcp\n'
    if f'      - {STAGING}\n' in text:
        return text
    if marker not in text:
        raise SystemExit(f'{path}: formal branch marker not found')
    return text.replace(marker, marker + f'      - {STAGING}\n', 1)


def patch_autotools(text: str, path: Path) -> str:
    lines = text.splitlines(keepends=True)
    changed_configure = 0
    changed_gcc = 0
    for i, line in enumerate(lines):
        if ('configure' in line and '--enable-dtls13' in line and
                '--disable-shared' in line and '--enable-static' in line and MACRO not in line):
            nl = '\n' if line.endswith('\n') else ''
            body = line[:-1] if nl else line
            if "CFLAGS='" in body:
                body = body.replace("CFLAGS='", f"CFLAGS='-D{MACRO} ", 1)
            elif body.rstrip().endswith(')'):
                pos = body.rfind(')')
                body = body[:pos] + f" CFLAGS='-O2 -D{MACRO}'" + body[pos:]
            else:
                body += f" CFLAGS='-O2 -D{MACRO}'"
            lines[i] = body + nl
            changed_configure += 1

    i = 0
    while i < len(lines):
        if lines[i].lstrip().startswith('gcc '):
            j = i
            command = lines[j]
            while command.rstrip().endswith('\\') and j + 1 < len(lines):
                j += 1
                command += lines[j]
            if 'native/dtls/wbd_dtls_shim.c' in command and MACRO not in command:
                indent = lines[i][:len(lines[i]) - len(lines[i].lstrip())]
                rest = lines[i].lstrip()
                lines[i] = indent + rest.replace('gcc ', f'gcc -D{MACRO} ', 1)
                changed_gcc += 1
            i = j
        i += 1

    out = ''.join(lines)
    if 'native/dtls/wbd_dtls_shim.c' in out:
        if MACRO not in out:
            raise SystemExit(f'{path}: shim build still lacks replay-window macro')
        if changed_configure == 0 and changed_gcc == 0 and MACRO not in text:
            raise SystemExit(f'{path}: shim build detected but no autotools/gcc build command patched')
    return out


detected = []
for path in sorted(ROOT.glob('*.yml')):
    text = path.read_text()
    if 'native/dtls/wbd_dtls_shim.c' not in text and 'native\\dtls\\wbd_dtls_shim.c' not in text:
        continue
    detected.append(path.name)
    if path.name == 'windows-portable-bundle.yml':
        text = replace_exact(
            text,
            "      - 'cmd/wbd-faketcp/**'\n",
            "      - 'cmd/wbd-faketcp/**'\n      - 'internal/faketcp/**'\n",
            count=2,
            path=path,
        )
        text = add_staging_branch(text, path)
        text = replace_exact(
            text,
            '          cmake -S . -B build -DWOLFSSL_DTLS13=yes -DWOLFSSL_DTLS=yes -DWOLFSSL_EXAMPLES=no -DWOLFSSL_CRYPT_TESTS=no -DBUILD_SHARED_LIBS=OFF -DCMAKE_POLICY_DEFAULT_CMP0091=NEW -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded\n',
            "          cmake -S . -B build -DWOLFSSL_DTLS13=yes -DWOLFSSL_DTLS=yes -DWOLFSSL_EXAMPLES=no -DWOLFSSL_CRYPT_TESTS=no -DBUILD_SHARED_LIBS=OFF -DCMAKE_POLICY_DEFAULT_CMP0091=NEW -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded '-DCMAKE_C_FLAGS=/DWOLFSSL_DTLS_WINDOW_WORDS=128'\n",
            path=path,
        )
        text = replace_exact(
            text,
            '          cl /nologo /O2 /MT /Iupstream-wolfssl /Iupstream-wolfssl\\build /Iupstream-wolfssl\\wolfssl native\\dtls\\wbd_dtls_shim.c /Fe:build\\windows-runtime\\wbd_dtls_shim.exe /link upstream-wolfssl\\build\\Release\\wolfssl.lib ws2_32.lib crypt32.lib advapi32.lib\n',
            '          cl /nologo /O2 /MT /DWOLFSSL_DTLS_WINDOW_WORDS=128 /Iupstream-wolfssl /Iupstream-wolfssl\\build /Iupstream-wolfssl\\wolfssl native\\dtls\\wbd_dtls_shim.c /Fe:build\\windows-runtime\\wbd_dtls_shim.exe /link upstream-wolfssl\\build\\Release\\wolfssl.lib ws2_32.lib crypt32.lib advapi32.lib\n',
            path=path,
        )
        text = replace_exact(
            text,
            '          if ($LASTEXITCODE -ne 0) { throw "DTLS shim build failed: $LASTEXITCODE" }\n',
            '          if ($LASTEXITCODE -ne 0) { throw "DTLS shim build failed: $LASTEXITCODE" }\n          Write-Output \'WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096\'\n',
            path=path,
        )
    else:
        text = patch_autotools(text, path)
        if path.name == 'reconnect-netem.yml':
            text = add_staging_branch(text, path)
            if 'WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096' not in text:
                needle = '            native/dtls/wbd_dtls_shim.c /tmp/wolf/build/src/.libs/libwolfssl.a -lm -o "$ASSET/wbd_dtls_shim"\n'
                text = replace_exact(text, needle, needle + "          echo 'WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096'\n", path=path)
    path.write_text(text)

# Linux release builds the same asserted shim through scripts/build_linux_server_bundle.sh.
linux_release = ROOT / 'linux-server-release.yml'
linux_text = linux_release.read_text()
linux_text = add_staging_branch(linux_text, linux_release)
linux_release.write_text(linux_text)

if not detected:
    raise SystemExit('no workflow DTLS shim build entrypoints detected')

for path in sorted(ROOT.glob('*.yml')):
    text = path.read_text()
    if 'native/dtls/wbd_dtls_shim.c' in text or 'native\\dtls\\wbd_dtls_shim.c' in text:
        if MACRO not in text:
            raise SystemExit(f'{path}: replay-window macro missing after patch')

print('WBD_STAGING_DTLS_WORKFLOW_PATCH_PASS detected=' + ','.join(detected))
