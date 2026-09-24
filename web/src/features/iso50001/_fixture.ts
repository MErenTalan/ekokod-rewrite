import type { MeResponse } from '@/lib/api/errors';
import type { ISOClauses, ISOFile, ISONote, ISOProject } from '@/lib/api/types';
import fixture from '@/lib/session/permissions.fixture.json';

export const BUILDING = 'b-1';

export const me = (role: keyof typeof fixture): MeResponse => ({
  id: `u-${role}`,
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
});

export const clauses: ISOClauses = {
  items: [
    { id: '5', title: '5. Liderlik', subs: [
      { id: '5.1', title: '5.1 Liderlik ve Taahhüt', description: 'Üst yönetim, EnYS’nin etkililiği için liderlik göstermelidir.' },
      { id: '5.2', title: '5.2 Enerji Politikası', description: 'Üst yönetim bir enerji politikası oluşturmalıdır.' },
    ] },
    { id: '6', title: '6. Planlama', subs: [
      { id: '6.3', title: '6.3 Enerji Gözden Geçirmesi', description: 'Kuruluş önemli enerji kullanımlarını belirlemelidir.', template: 'significant-energy-uses' },
    ] },
    { id: '7', title: '7. Destek', subs: [{ id: '7.1', title: '7.1 Kaynaklar', description: 'Kaynaklar sağlanmalıdır.' }] },
    { id: '8', title: '8. Operasyon', subs: [{ id: '8.1', title: '8.1 Operasyonel Planlama', description: 'Prosesler kontrol edilmelidir.' }] },
    { id: '9', title: '9. Performans Değerlendirmesi', subs: [{ id: '9.1', title: '9.1 İzleme', description: 'Performans izlenmelidir.' }] },
  ],
};

const ids = ['5.1', '5.2', '5.3', '6.1', '6.2', '6.3', '6.4', '6.5', '6.6', '7.1', '7.2', '7.3', '7.4', '7.5', '8.1', '8.2', '8.3', '9.1', '9.2', '9.3'];

export const project: ISOProject = {
  progress: 15,
  gantt_available: true,
  project_start: '2026-01-01',
  project_end: '2026-12-31',
  clauses: [
    { clause_id: '5', start: '2026-01-01', end: '2026-03-31', status: 'completed' },
    { clause_id: '6', start: '2026-04-01', end: '2026-08-31', status: 'in_progress' },
    { clause_id: '7', start: '2026-05-01', end: '2026-09-30', status: 'not_started' },
    { clause_id: '8', start: '2026-07-01', end: '2026-10-31', status: 'not_started' },
    { clause_id: '9', start: '2026-01-01', end: '2026-06-14', status: 'expired' },
  ],
  counts: ids.map((clause_id) => ({ clause_id, notes: clause_id === '5.1' ? 2 : 0, files: clause_id === '5.2' ? 1 : 0 })),
};

export const emptyProject: ISOProject = {
  progress: 0,
  gantt_available: false,
  clauses: ['5', '6', '7', '8', '9'].map((clause_id) => ({ clause_id })),
  counts: ids.map((clause_id) => ({ clause_id, notes: 0, files: 0 })),
};

export const note = (over: Partial<ISONote> = {}): ISONote => ({
  id: 'n-1', clause_id: '5.1', title: 'Politika', body: 'Enerji politikası yayımlandı.', created_at: '2026-06-01T09:00:00+03:00',
  updated_at: '2026-06-01T09:00:00+03:00', ...over,
});

export const file = (over: Partial<ISOFile> = {}): ISOFile => ({
  id: 'f-1', clause_id: '5.1', name: 'Politika.pdf', content_type: 'application/pdf', size_bytes: 20480, created_at: '2026-06-01T09:00:00+03:00', ...over,
});
