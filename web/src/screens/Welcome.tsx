import { Arrow, Mark } from '../components'
import type { BootMode } from '../model'
export function Welcome({ choose, hasShips, back }: { choose: (mode: BootMode) => void; hasShips: boolean; back: () => void }) {
  return <section className="welcome" aria-labelledby="welcome-title">
    <Mark size={132}/><h1 id="welcome-title">Welcome to Harbor</h1>
    <div className="welcome-choices">
      <button className="choice" onClick={() => choose('keyfile')} aria-label="Boot from a keyfile"><strong>Boot from a keyfile</strong><span>Start your ship for the first time<br/>with its Urbit keyfile.</span></button>
      <button className="choice" onClick={() => choose('comet')} aria-label="Create a comet"><strong>Create a comet</strong><span>Generate a free identity<br/>and explore Urbit.</span></button>
      <button className="existing-choice" onClick={() => choose('existing')}>Use an existing pier <Arrow/></button>
    </div>
    {hasShips && <button className="text-button welcome-back" onClick={back}>← Your ships</button>}
  </section>
}
