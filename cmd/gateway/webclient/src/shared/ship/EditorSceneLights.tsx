import { useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import * as THREE from 'three';

/** Scene lighting matching the game (without draggable debug light). */
export function EditorSceneLights() {
  const ambientGroupRef = useRef<THREE.Group>(null);

  useFrame((state) => {
    if (ambientGroupRef.current) {
      ambientGroupRef.current.position.x = state.camera.position.x;
      ambientGroupRef.current.position.y = state.camera.position.y;
    }
  });

  return (
    <group ref={ambientGroupRef}>
      <ambientLight intensity={1.2} color="#ffffff" />
      <pointLight position={[10, 10, 5]} intensity={8.0} color="#aaddff" distance={300} decay={2} />
      <pointLight position={[-10, -10, 5]} intensity={8.0} color="#ffddaa" distance={300} decay={2} />
    </group>
  );
}
