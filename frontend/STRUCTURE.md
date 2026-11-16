frontend/
├── public/
│   ├── images/
│   └── favicon.ico
│
├── src/
│   ├── app/
│   │   ├── App.tsx
│   │   ├── router.tsx
│   │   ├── providers/
│   │   │   ├── ThemeProvider.tsx
│   │   │   └── AuthProvider.tsx
│   │   └── layouts/
│   │       ├── MainLayout.tsx
│   │       └── AuthLayout.tsx
│   │
│   ├── pages/
│   │   ├── Dashboard/
│   │   │   └── DashboardPage.tsx
│   │   ├── Datasets/
│   │   │   └── DatasetsPage.tsx
│   │   ├── Annotations/
│   │   │   └── AnnotationsPage.tsx
│   │   └── Settings/
│   │       └── SettingsPage.tsx
│   │
│   ├── features/
│   │   ├── datasets/
│   │   │   ├── components/
│   │   │   │   ├── DatasetCard.tsx
│   │   │   │   ├── DatasetFilters.tsx
│   │   │   │   └── DatasetList.tsx
│   │   │   ├── hooks/
│   │   │   │   └── useDatasets.ts
│   │   │   ├── services/
│   │   │   │   └── datasetService.ts
│   │   │   ├── types/
│   │   │   │   └── dataset.types.ts
│   │   │   └── index.ts
│   │   │
│   │   └── annotations/
│   │       ├── components/
│   │       ├── hooks/
│   │       ├── services/
│   │       ├── types/
│   │       └── index.ts
│   │
│   ├── components/
│   │   ├── ui/
│   │   │   ├── Button/
│   │   │   ├── Input/
│   │   │   ├── Modal/
│   │   │   └── Card/
│   │   │
│   │   └── common/
│   │       ├── Header/
│   │       ├── Sidebar/
│   │       ├── Loading/
│   │       └── EmptyState/
│   │
│   ├── hooks/
│   │   ├── useDebounce.ts
│   │   └── useLocalStorage.ts
│   │
│   ├── services/
│   │   ├── api.ts
│   │   └── authService.ts
│   │
│   ├── types/
│   │   ├── api.types.ts
│   │   └── common.types.ts
│   │
│   ├── utils/
│   │   ├── formatDate.ts
│   │   └── formatFileSize.ts
│   │
│   ├── assets/
│   │   ├── images/
│   │   └── icons/
│   │
│   ├── styles/
│   │   ├── globals.css
│   │   └── variables.css
│   │
│   └── main.tsx
│
├── .env
├── package.json
└── tsconfig.json