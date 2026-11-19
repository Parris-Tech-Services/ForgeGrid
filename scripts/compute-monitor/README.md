# ForgeGrid Compute Monitor — recovered deployed source

This directory tracks the **actual deployed ForgeGrid Compute Monitor** recovered from **Laptop02** on **2026-09-10** so future agents do not have to reconstruct it from chat history.

## What this is

The deployed monitor is a small PowerShell WinForms GUI titled **`ForgeGrid Compute Monitor`**. It is separate from the ForgeGrid worker process itself.

Recovered live deployment path on Laptop02:

```text
C:\ProgramData\ForeGrid\ForeGridComputeMonitor.ps1
```

Recovered startup launcher:

```text
LaunchForeGridComputeMonitor.vbs
```

The VBS launches the PowerShell host hidden while the WinForms GUI remains visible:

```text
powershell.exe -NoProfile -ExecutionPolicy Bypass -STA -WindowStyle Hidden -File "C:\ProgramData\ForeGrid\ForeGridComputeMonitor.ps1"
```

A `StartForgeGridComputeMonitor.bat` file was also present in the recovered ProgramData/Startup bundle and is preserved here exactly as recovered.

## Current UI behaviour in the recovered source

The monitor:

- creates a `420 x 230` WinForms window;
- sets the title to `ForeGrid Compute Monitor`;
- is `TopMost` and centred on screen;
- refreshes every 1 second;
- finds running `ForgeGrid` processes;
- reports ForgeGrid process count;
- estimates ForgeGrid CPU usage;
- reports ForeGrid RAM use;
- shows ForgeGrid PIDs;
- shows `worker not currently running` when no ForgeGrid process exists.

The recovered version **does not contain temperature-sensor code yet**.

## Recovered file hashes from Laptop02

These SHA-256 hashes describe the files as recovered from Laptop02 before adding them to Git:

```text
d3cf8249ab2d4fd68aa6c8beb6cc7d6bb30cbb35b612e39a3068edbb6ccc868b  ForgeGridComputeMonitor.ps1
a4235e3209884c19ee4f4b9f60bf54da915febf38b5dbfc4b4e67037c04e8ac5  LaunchForeGridComputeMonitor.vbs
3b0fa7fc3e36692873cbf94d24637fa38fee299868d55fedd181e06bedba473b  StartForgeGridComputeMonitor.bat
```

Note: Git/text tooling may normalise BOM or line endings after import. The hashes above are the hashes of the files recovered from the physical Laptop02 bundle.

## Important fleet context

Josh reports that an app titled **ForgeGrid Compute Monitor** appears automatically on all 12 ForgeGrid computers at startup. As of this recovery, only Laptop02's deployed source has been directly captured and archived here. Do **not** assume all machines are byte-identical until they are compared through Action1.

Recommended fleet verification:

1. query `C:\ProgramData\ForeGrid\ForeGridComputeMonitor.ps1` on Laptop01–Laptop10 and JPARRIS;
2. compare SHA-256 values;
3. inspect the Fedora/AVANCE-WS7 equivalent separately;
4. choose one canonical source;
5. make monitor changes in Git first;
6. deploy through Action1 with a canary before fleet-wide rollout.

## Next planned enhancement

Add live temperature/thermal-state reporting to this existing monitor rather than inventing a second monitor UI. The current ForgeGrid Worker Console design already calls for resource temperature when available.

Desired outcome:

```text
Host: Laptop02
ForgeGrid processes: 1
ForeGrid CPU: ...
ForeGrid RAM: ...
CPU Temp: 67 C  NORMAL
PIDs: ...
```

Temperature must be optional on machines without a trustworthy sensor. Do not treat missing sensor data as `0 C`.

## Source-of-truth rule for future agents

If chat memory and this repository disagree about the Compute Monitor, start from:

1. the tracked files in this directory;
2. then compare against the live deployed files under `C:\ProgramData\ForeGrid` using Action1.

Do not recreate the app from memory unless the tracked and live copies are both unavailable.
