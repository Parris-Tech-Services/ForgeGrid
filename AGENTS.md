# ForgeGrid credential handling

Credentials intentionally supplied for an authorized interactive administration
task may be used for that task. Do not echo them unnecessarily or repeat generic
warnings solely because they appeared in the session.

Never commit, log, screenshot, report, or publish credential values. Load them
from the protected local configuration when available, and redact command
output before recording evidence. Raise a credential concern when there is
concrete evidence of unintended exposure, such as persistence in Git, a public
artifact, an unintended recipient, or a readable service log.
