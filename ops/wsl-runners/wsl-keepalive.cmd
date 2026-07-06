@echo off
REM WSL2 persistent session holder.
REM
REM The naive "wsl.exe -d Ubuntu bash -c 'while true; do sleep 3600; done'"
REM version is fragile: when WSL terminates the instance (e.g. after the
REM last wsl.exe session ends, which systemd-logind treats as "idle" and
REM poweroffs), the wsl.exe process is signalled (STATUS_CONTROL_C_EXIT /
REM 0xC000013A) and the holder dies. The VM then boots fresh with no
REM holder, the last transient wsl.exe session ends, and the cycle
REM repeats every ~1 minute.
REM
REM This launcher detaches a nohup sleep INSIDE the Ubuntu instance so it
REM survives the wsl.exe process exiting, then keeps a foreground wsl.exe
REM sleep alive too. The detached sleep keeps the VM running; the
REM foreground sleep keeps a logind session open. Together they hold
REM the VM continuously even across WSL poweroff/reboot cycles.
wsl.exe -d Ubuntu -- bash -lc "nohup bash -c 'while true; do sleep 3600; done' >/dev/null 2>&1 & disown; echo keepalive-session-launched"
wsl.exe -d Ubuntu -- bash -c "while true; do sleep 3600; done"
