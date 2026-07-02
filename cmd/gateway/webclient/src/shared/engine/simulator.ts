import * as THREE from 'three';
import type { EngineDefinition, EngineModeId } from '@/src/shared/types/engine';
import { createParticleTextureFromConfig } from './particleTexture';

export interface WorldTransform {
  x: number;
  y: number;
  z: number;
  rotation: number;
}

interface Particle {
  position: THREE.Vector3;
  velocity: THREE.Vector3;
  life: number;
  maxLife: number;
  scale: number;
}

export class EngineSimulator {
  private config: EngineDefinition;
  private particles: Particle[] = [];
  private emitAccumulator = 0;
  private texture: THREE.CanvasTexture;
  private lightIntensity = 0;
  private readonly dummy = new THREE.Object3D();

  constructor(config: EngineDefinition) {
    this.config = config;
    this.texture = createParticleTextureFromConfig(config.flame);
    this.initParticles();
  }

  private initParticles() {
    const max = this.config.particle.maxCount;
    this.particles = Array.from({ length: max }, () => ({
      position: new THREE.Vector3(0, 0, -1000),
      velocity: new THREE.Vector3(0, 0, 0),
      life: 0,
      maxLife: 0.3,
      scale: 1,
    }));
  }

  setConfig(config: EngineDefinition) {
    this.config = config;
    this.rebuildTexture();
    if (this.particles.length !== config.particle.maxCount) {
      this.initParticles();
    }
  }

  getConfig(): EngineDefinition {
    return this.config;
  }

  getTexture(): THREE.CanvasTexture {
    return this.texture;
  }

  rebuildTexture() {
    this.texture.dispose();
    this.texture = createParticleTextureFromConfig(this.config.flame);
  }

  getLightIntensity(): number {
    return this.lightIntensity;
  }

  tick(delta: number, transform: WorldTransform, activeMode: EngineModeId) {
    const mode = this.config.modes[activeMode];
    const mount = this.config.mount;
    const pCfg = this.config.particle;
    const pl = this.config.pointLight;

    if (pl.enabled) {
      const isActive = activeMode === 'thrust' || activeMode === 'reverse';
      if (isActive) {
        const target = pl.intensityMin + Math.random() * (pl.intensityMax - pl.intensityMin);
        this.lightIntensity = THREE.MathUtils.lerp(this.lightIntensity, target, pl.rampUp);
      } else {
        this.lightIntensity = THREE.MathUtils.lerp(this.lightIntensity, 0, pl.rampDown);
      }
    } else {
      this.lightIntensity = 0;
    }

    if (!mode || mode.emitRate <= 0 || mode.poolWeight <= 0) {
      this.emitAccumulator = 0;
      this.updateParticles(delta);
      return;
    }

    const targetEmitRate = mode.emitRate;
    this.emitAccumulator += delta * targetEmitRate;
    const emitCount = Math.floor(this.emitAccumulator);
    this.emitAccumulator -= emitCount;

    const shipRot = transform.rotation + mount.exhaustAngle;
    const dirX = Math.cos(shipRot);
    const dirY = Math.sin(shipRot);
    const rightX = dirY;
    const rightY = -dirX;
    const lateralOffset = mount.lateralOffset + mount.localPosition.y;

    let emitted = 0;
    for (let i = 0; i < this.particles.length; i++) {
      if (this.particles[i].life <= 0 && emitted < emitCount) {
        const maxLifeVal = Math.random() * (mode.lifeMax - mode.lifeMin) + mode.lifeMin;
        this.particles[i].life = this.particles[i].maxLife = Math.max(0.001, maxLifeVal);

        const spread = pCfg.positionSpread;
        const spreadX = (Math.random() - 0.5) * spread;
        const spreadY = (Math.random() - 0.5) * spread;

        const originX = transform.x + mount.localPosition.x;
        const originY = transform.y + mount.localPosition.y;

        this.particles[i].position.set(
          originX - dirX * pCfg.backOffset + rightX * lateralOffset + spreadX,
          originY - dirY * pCfg.backOffset + rightY * lateralOffset + spreadY,
          transform.z + mount.localPosition.z + pCfg.zOffset
        );

        const speed = Math.random() * mode.speedVar + mode.speedBase;
        const lateralVelocity = (Math.random() - 0.5) * mode.lateralVelocity;

        this.particles[i].velocity.set(
          -dirX * speed + rightX * lateralVelocity,
          -dirY * speed + rightY * lateralVelocity,
          0
        );

        emitted++;
      }
    }

    this.updateParticles(delta);
  }

  private updateParticles(delta: number) {
    for (const p of this.particles) {
      if (p.life > 0) {
        p.life -= delta;
        p.position.addScaledVector(p.velocity, delta);
        p.scale = p.life / p.maxLife;
        if (p.life <= 0) {
          p.position.set(0, 0, -1000);
        }
      }
    }
  }

  writeInstanceMatrices(mesh: THREE.InstancedMesh) {
    const count = this.particles.length;
    for (let i = 0; i < count; i++) {
      const p = this.particles[i];
      this.dummy.position.copy(p.position);
      const s = Math.max(0, p.scale);
      this.dummy.scale.setScalar(s);
      this.dummy.updateMatrix();
      mesh.setMatrixAt(i, this.dummy.matrix);
    }
    mesh.instanceMatrix.needsUpdate = true;
  }

  dispose() {
    this.texture.dispose();
  }
}
