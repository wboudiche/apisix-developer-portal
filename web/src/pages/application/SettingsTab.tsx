import { useEffect, useState } from 'react'
import type { Application } from '../../api/types'
import type { ModalSpec } from '../../components/ConfirmModal'

function CheckIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
      <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function SettingsTab({ app, notify, openModal }: {
  app: Application
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
}) {
  const [name, setName] = useState(app.name)
  const [desc, setDesc] = useState(app.description)

  // The page shell keeps this mounted while the switcher navigates between
  // apps — resync the form when the displayed application changes.
  useEffect(() => { setName(app.name); setDesc(app.description) }, [app.id, app.name, app.description])

  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch"><h3>Détails de l'application</h3></div>
        <div className="cb">
          <div className="field">
            <label htmlFor="s-name">Nom de l'application</label>
            <input id="s-name" type="text" value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="s-desc">Description</label>
            <textarea id="s-desc" value={desc} onChange={e => setDesc(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="s-env">Environnement par défaut</label>
            <select id="s-env" defaultValue="Production">
              <option>Production</option>
              <option>Sandbox</option>
            </select>
            <p className="hint">Détermine la clé pré-sélectionnée dans les exemples de code.</p>
          </div>
          {/* DEMO: no application-update endpoint yet */}
          <button className="btn primary" onClick={() => notify('Modifications enregistrées (démo)')}>
            <CheckIcon />Enregistrer
          </button>
        </div>
      </div>

      <p className="section-title" style={{ marginTop: 26 }}>Zone sensible</p>
      <div className="danger-zone">
        <div className="dz-t">
          <h4>Supprimer cette application</h4>
          <p>Révoque toutes les clés et résilie les abonnements. Irréversible.</p>
        </div>
        {/* Blueprint demo behavior — no delete endpoint yet */}
        <button
          className="btn danger"
          onClick={() => openModal({
            title: `Supprimer « ${app.name} » ?`, danger: true, confirmLabel: 'Supprimer définitivement',
            body: 'Toutes les clés seront révoquées et les abonnements résiliés dans APISIX. Cette action est irréversible.',
            onConfirm: () => notify('Application supprimée (démo)'),
          })}
        >
          <TrashIcon />Supprimer l'application
        </button>
      </div>
    </section>
  )
}
