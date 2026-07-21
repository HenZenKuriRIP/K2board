declare module 'globe.gl' {
  import type { Object3D } from 'three'

  export interface GlobeInstance {
    (el: HTMLElement): GlobeInstance
    globeImageUrl(url: string): GlobeInstance
    bumpImageUrl(url: string): GlobeInstance
    backgroundImageUrl(url: string): GlobeInstance
    showAtmosphere(show: boolean): GlobeInstance
    atmosphereColor(c: string): GlobeInstance
    atmosphereAltitude(a: number): GlobeInstance
    pointsData(data: any[]): GlobeInstance
    pointLat(fn: any): GlobeInstance
    pointLng(fn: any): GlobeInstance
    pointColor(fn: any): GlobeInstance
    pointAltitude(fn: any): GlobeInstance
    pointRadius(fn: any): GlobeInstance
    pointLabel(fn: any): GlobeInstance
    pointsMerge(v: boolean): GlobeInstance
    arcsData(data: any[]): GlobeInstance
    arcStartLat(fn: string | ((d: any) => number)): GlobeInstance
    arcStartLng(fn: string | ((d: any) => number)): GlobeInstance
    arcEndLat(fn: string | ((d: any) => number)): GlobeInstance
    arcEndLng(fn: string | ((d: any) => number)): GlobeInstance
    arcColor(fn: string | string[] | ((d: any) => string | string[])): GlobeInstance
    arcAltitude(fn: number | string | ((d: any) => number)): GlobeInstance
    arcStroke(fn: number | string | ((d: any) => number | null)): GlobeInstance
    arcDashLength(n: number): GlobeInstance
    arcDashGap(n: number): GlobeInstance
    arcDashAnimateTime(n: number | ((d: any) => number)): GlobeInstance
    ringsData(data: any[]): GlobeInstance
    ringLat(fn: string | ((d: any) => number)): GlobeInstance
    ringLng(fn: string | ((d: any) => number)): GlobeInstance
    ringColor(fn: string | ((d: any) => string | string[])): GlobeInstance
    ringMaxRadius(n: number): GlobeInstance
    ringPropagationSpeed(n: number): GlobeInstance
    ringRepeatPeriod(n: number): GlobeInstance
    labelsData(data: any[]): GlobeInstance
    labelLat(fn: string | ((d: any) => number)): GlobeInstance
    labelLng(fn: string | ((d: any) => number)): GlobeInstance
    labelText(fn: string | ((d: any) => string)): GlobeInstance
    labelSize(fn: number | ((d: any) => number)): GlobeInstance
    labelDotRadius(fn: number | ((d: any) => number)): GlobeInstance
    labelColor(fn: string | ((d: any) => string)): GlobeInstance
    labelAltitude(fn: number | ((d: any) => number)): GlobeInstance
    labelResolution(n: number): GlobeInstance
    width(n: number): GlobeInstance
    height(n: number): GlobeInstance
    backgroundColor(c: string): GlobeInstance
    controls(): { autoRotate: boolean; autoRotateSpeed: number; enableZoom: boolean }
    pointOfView(pov: { lat?: number; lng?: number; altitude?: number }, ms?: number): GlobeInstance
    onPointClick(fn: (p: any) => void): GlobeInstance
    onGlobeReady(fn: () => void): GlobeInstance
    scene(): Object3D
    renderer(): { domElement: HTMLCanvasElement; setPixelRatio(n: number): void }
    pauseAnimation(): GlobeInstance
    resumeAnimation(): GlobeInstance
    _destructor(): void
  }

  export default function Globe(): GlobeInstance
}
