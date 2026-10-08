'use client';

import { createContext, useContext, type ReactNode } from 'react';

const TileUrl = createContext('');

/** Hands the server-read tile source (lib/map-config) to every client map. */
export function MapConfigProvider({ tileUrl, children }: { tileUrl: string; children: ReactNode }) {
  return <TileUrl.Provider value={tileUrl}>{children}</TileUrl.Provider>;
}

export const useMapTileUrl = () => useContext(TileUrl);
