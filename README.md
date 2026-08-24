# wobuzhidao

Personal experimental VPN transport project.

The project goal is to build a user-space, UDP-like logical transport over multiple independent **real kernel TCP** carriers, while keeping stock VLESS + XTLS Vision + REALITY on each public carrier. The project is for personal use and experimentation; protocol correctness, reproducible testing, and cross-session engineering continuity are first-class requirements.

Start from `FRESH_AGENT_BOOTSTRAP.md` once the development control plane is initialized.
