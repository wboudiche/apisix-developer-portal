export const fr = {
  test: { hello: 'Bonjour {name}' },
  common: { loading: 'Chargement…', save: 'Enregistrer', cancel: 'Annuler', delete: 'Supprimer', add: 'Ajouter' },
} as const

export type Messages = typeof fr
