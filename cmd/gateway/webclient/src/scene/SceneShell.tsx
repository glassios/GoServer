// @ts-nocheck
// Reusable scene furniture for the GoServer 3D client, extracted from the
// SpaceShip2d sandbox App.tsx: orthographic camera controller (wheel/pinch zoom),
// parallax starfield, drifting asteroids, and the lighting rig.
import { useFrame, useThree } from '@react-three/fiber';
import { Stars } from '@react-three/drei';
import { useEffect, useMemo, useRef, useState } from 'react';
import * as THREE from 'three';
import { assetUrl } from '@/src/shared/assetUrl';

export function Lights() {
  const ambientGroupRef = useRef<THREE.Group>(null);

  useFrame((state) => {
    if (ambientGroupRef.current) {
      ambientGroupRef.current.position.x = state.camera.position.x;
      ambientGroupRef.current.position.y = state.camera.position.y;
    }
  });

  return (
    <group ref={ambientGroupRef}>
      <ambientLight intensity={1.2} color={'#ffffff'} />
      <pointLight position={[10, 10, 5]} intensity={8.0} color="#aaddff" distance={300} decay={2} />
      <pointLight position={[-10, -10, 5]} intensity={8.0} color="#ffddaa" distance={300} decay={2} />
    </group>
  );
}

export function EndlessBackground() {
  const groupRef = useRef<THREE.Group>(null);

  useFrame((state) => {
    if (!groupRef.current) return;
    // Parallax: stars trail the camera slightly slower than the world.
    groupRef.current.position.x = state.camera.position.x * 0.9;
    groupRef.current.position.y = state.camera.position.y * 0.9;
  });

  return (
    <group ref={groupRef}>
      <Stars radius={100} depth={50} count={7000} factor={6} saturation={0} fade speed={1.5} />
    </group>
  );
}

export function Asteroids() {
  const [textures, setTextures] = useState<{
    diffuse?: THREE.Texture;
    normal?: THREE.Texture;
    loaded: boolean;
    error: boolean;
  }>({ loaded: false, error: false });

  useEffect(() => {
    const loader = new THREE.TextureLoader();
    Promise.all([
      new Promise<THREE.Texture>((resolve, reject) => loader.load(assetUrl('/aster.png'), resolve, undefined, reject)),
      new Promise<THREE.Texture>((resolve, reject) => loader.load(assetUrl('/astern.png'), resolve, undefined, reject)),
    ])
      .then(([diffuse, normal]) => setTextures({ diffuse, normal, loaded: true, error: false }))
      .catch((err) => {
        console.error('Failed to load asteroid textures.', err);
        setTextures({ loaded: true, error: true });
      });
  }, []);

  const asteroidData = useMemo(
    () =>
      Array.from({ length: 1 }).map(() => ({
        position: [(Math.random() - 0.5) * 10 + 5, (Math.random() - 0.5) * 10, -2] as [number, number, number],
        size: Math.random() * 5 + 3,
        rotationSpeed: (Math.random() - 0.5) * 0.5,
        initialRotation: Math.random() * Math.PI * 2,
      })),
    []
  );

  const groupRef = useRef<THREE.Group>(null);

  useFrame((_, delta) => {
    if (!groupRef.current) return;
    groupRef.current.children.forEach((child, index) => {
      const data = asteroidData[index];
      if (data) child.rotation.z += data.rotationSpeed * delta;
    });
  });

  if (!textures.loaded || textures.error) return null;

  return (
    <group ref={groupRef}>
      {asteroidData.map((data, i) => (
        <mesh key={i} position={data.position} rotation={[0, 0, data.initialRotation]}>
          <planeGeometry args={[data.size * (1280 / 699), data.size]} />
          <meshStandardMaterial
            map={textures.diffuse}
            normalMap={textures.normal}
            transparent
            alphaTest={0.1}
            roughness={0.8}
            metalness={0.2}
          />
        </mesh>
      ))}
    </group>
  );
}

const MIN_CAMERA_ZOOM = 10;
const MAX_CAMERA_ZOOM = 200;
const ZOOM_SMOOTH_SPEED = 10;

export function CameraController() {
  const { camera, gl } = useThree();
  const targetZoom = useRef((camera as THREE.OrthographicCamera).zoom);

  useEffect(() => {
    targetZoom.current = (camera as THREE.OrthographicCamera).zoom;
    let initialPinchDistance: number | null = null;
    let initialZoom = targetZoom.current;

    const clampZoom = (zoom: number) => Math.max(MIN_CAMERA_ZOOM, Math.min(zoom, MAX_CAMERA_ZOOM));

    const onWheel = (e: WheelEvent) => {
      targetZoom.current = clampZoom(targetZoom.current * Math.pow(0.999, e.deltaY * 0.5));
      e.preventDefault();
    };
    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length === 2) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        initialPinchDistance = Math.sqrt(dx * dx + dy * dy);
        initialZoom = targetZoom.current;
      }
    };
    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length === 2 && initialPinchDistance !== null) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        const distance = Math.sqrt(dx * dx + dy * dy);
        targetZoom.current = clampZoom(initialZoom * (distance / initialPinchDistance));
        e.preventDefault();
      }
    };
    const onTouchEnd = (e: TouchEvent) => {
      if (e.touches.length < 2) initialPinchDistance = null;
    };

    const el = gl.domElement;
    el.addEventListener('wheel', onWheel, { passive: false });
    el.addEventListener('touchstart', onTouchStart, { passive: false });
    el.addEventListener('touchmove', onTouchMove, { passive: false });
    el.addEventListener('touchend', onTouchEnd, { passive: false });
    el.addEventListener('touchcancel', onTouchEnd, { passive: false });
    return () => {
      el.removeEventListener('wheel', onWheel);
      el.removeEventListener('touchstart', onTouchStart);
      el.removeEventListener('touchmove', onTouchMove);
      el.removeEventListener('touchend', onTouchEnd);
      el.removeEventListener('touchcancel', onTouchEnd);
    };
  }, [camera, gl.domElement]);

  useFrame((_, delta) => {
    const cam = camera as THREE.OrthographicCamera;
    const smoothFactor = 1 - Math.exp(-ZOOM_SMOOTH_SPEED * delta);
    const nextZoom = THREE.MathUtils.lerp(cam.zoom, targetZoom.current, smoothFactor);
    if (Math.abs(nextZoom - cam.zoom) > 0.001) {
      cam.zoom = nextZoom;
      cam.updateProjectionMatrix();
    }
  });

  return null;
}
