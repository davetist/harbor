export type ShipState = 'stopped' | 'starting' | 'running' | 'stopping' | 'maintaining' | 'externally_running' | 'failed'
export type Operation = 'start' | 'stop' | 'pack' | 'meld' | 'roll' | 'chop' | 'update' | 'replay' | 'backup'
export interface Ship { id: string; identity?: string; name: string; nickname: string; pierPath: string; runtimePath: string; loomExponent: number; httpBindAddress: string; webPort: number; desiredRunning: boolean; recoveryBlocked: boolean; recoveryBlockReason: string; state: ShipState; error: string; pierBytes?: number; measuredAt?: string; runtimeVersion?: string }
export interface Host { hostname: string; platform: string; dataDir: string; pierRoot?: string; allowedRoots?: string[]; timezone: string; loomSizes: number[]; defaultShipHTTPBindAddress: string; maintenanceProfile: string; maintenanceValidated: boolean }
export interface Schedule { id: string; shipId: string; kind: Operation; intervalDays: number; localTime: string; timezone: string; enabled: boolean; nextDue: string; lastResult: string; afterScheduleId?: string; anchorDate?: string }
export interface Job { id: string; shipId: string; kind: string; phase: string; result?: string; error: string; startedAt: string; retryable?: boolean }
export type BootMode = 'comet' | 'keyfile' | 'existing'
export interface BootRequest { mode: BootMode; folderName: string; name: string; pierPath: string; loomExponent: number; uploadId?: string; importArchive?: boolean; loomConfirmed?: boolean }
export interface BackupRecord { id: string; shipId: string; createdAt: string; verified: boolean }
export interface HarborClient {
  jobs(id: string): Promise<Job[]>; jobLogs(id: string): Promise<string>;
  retryBoot(id: string, file?: File): Promise<string>;
  verifyRecovery(id: string): Promise<void>;
  subscribe?(refresh: () => void): () => void;
  backups(id: string): Promise<BackupRecord[]>;
  host(): Promise<Host>; listShips(): Promise<Ship[]>; ship(id: string): Promise<Ship>;
  boot(request: BootRequest, file?: File): Promise<string>; action(id: string, operation: Operation): Promise<string>;
  job(id: string): Promise<Job>; schedules(id: string): Promise<Schedule[]>; saveSchedule(schedule: Schedule): Promise<void>;
  resize(id: string, rows: number, cols: number): Promise<void>; logs(id: string): Promise<string>; input(id: string, command: string): Promise<void>; accessCode(id: string): Promise<string>;
  openURL(id: string): Promise<string>; folders(path?: string): Promise<string[]>;
}
