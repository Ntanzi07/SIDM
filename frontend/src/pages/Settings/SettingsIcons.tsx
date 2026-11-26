// Os ícones do menu do Configurar: traço de 1,75 num quadro de 24, na cor do
// texto (currentColor), decorativos (o nome está no texto do item).

const PATHS = {
  account: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 20.5c.6-3.6 3.9-5.8 8-5.8s7.4 2.2 8 5.8" />
    </>
  ),
  shortcuts: (
    <>
      <rect height="12" rx="2.5" width="20" x="2" y="6" />
      <path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M7.5 14h9" />
    </>
  ),
  profiles: (
    <>
      <rect height="17" rx="2.5" width="15" x="4.5" y="4.5" />
      <path d="M9 3h6v3H9zM8.5 11h7M8.5 15h4.5" />
    </>
  ),
  equipments: (
    <>
      <rect height="10" rx="2" width="16" x="4" y="3" />
      <circle cx="9" cy="8" r="2" />
      <path d="M14 7h3M14 9.5h2M12 13v8M8 21h8" />
    </>
  ),
  cameras: (
    <>
      <path d="M3 8.5A2.5 2.5 0 0 1 5.5 6h1.8l1.9-2.2h5.6L16.7 6h1.8A2.5 2.5 0 0 1 21 8.5v9a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z" />
      <circle cx="12" cy="13" r="3.5" />
    </>
  ),
  imports: (
    <path d="M12 3.5v11.5M7.5 10.5l4.5 4.5 4.5-4.5M4.5 20.5h15" />
  ),
  users: (
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2.5 20c.5-3.3 3.1-5.3 6.5-5.3s6 2 6.5 5.3M15.5 4.7a3.5 3.5 0 0 1 0 6.6M18 14.9c2 .7 3.2 2.4 3.5 5.1" />
    </>
  ),
  storage: (
    <>
      <ellipse cx="12" cy="5.5" rx="8" ry="2.8" />
      <path d="M4 5.5v13c0 1.5 3.6 2.8 8 2.8s8-1.3 8-2.8v-13M4 12c0 1.5 3.6 2.8 8 2.8s8-1.3 8-2.8" />
    </>
  ),
}

export type SettingsIconName = keyof typeof PATHS

export function SettingsIcon({ name }: { name: SettingsIconName }) {
  return (
    <svg
      aria-hidden="true"
      className="settings-nav-icon"
      fill="none"
      focusable="false"
      height="18"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.75"
      viewBox="0 0 24 24"
      width="18"
    >
      {PATHS[name]}
    </svg>
  )
}
