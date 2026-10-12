import type { HarborUpdate } from './model'

export const harborReleasesURL = 'https://github.com/davetist/harbor/releases'

export function HarborVersion({ version, status, checking, ready, check }: { version: string; status?: HarborUpdate; checking: boolean; ready: boolean; check: () => void }) {
  if (ready && status?.available) return <a className="app-version update-available" href={harborReleasesURL} target="_blank" rel="noopener noreferrer" title={`Installed: Harbor ${version}. Open GitHub releases.`}>Harbor {status.latestVersion} available <span aria-hidden="true">↗</span></a>
  return <button className="text-button app-version" title="Check for updates" disabled={!ready || checking} onClick={check}>{checking ? 'Checking for updates…' : `Harbor ${version}`}</button>
}

export function HarborUpdateMessage({ status, error }: { status?: HarborUpdate; error: string }) {
  let message = error || status?.error || ''
  if (!message && status) {
    if (!status.latestVersion) message = 'No published installer is available for this host.'
    else if (!status.comparisonKnown) message = `Development build · latest release ${status.latestVersion}`
    else message = status.available ? 'A new version of Harbor is available.' : 'Harbor is up to date.'
  }
  return <p className={`harbor-update-result${error || status?.error ? ' error' : ''}`} role="status">{message}{!error && !status?.error && !status?.comparisonKnown && status?.releaseURL && <> · <a href={status.releaseURL} target="_blank" rel="noopener noreferrer">View release</a></>}</p>
}
