import { useEffect, useRef, useState, type DragEvent } from "react";

const hasFiles = (e: { dataTransfer: DataTransfer | null }) => !!e.dataTransfer?.types?.includes("Files");

/**
 * Arrastar arquivos do computador para uma área (o chat). `blocked` explica por que não dá para enviar agora (ou null).
 * Soltar sempre é tratado, mesmo bloqueado: senão o navegador abriria o arquivo e a pessoa sairia do app.
 */
export function useFileDrop({ blocked, onFiles }: { blocked: string | null; onFiles: (files: File[], problem?: string) => void }) {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0); // dragenter/dragleave disparam também ao passar pelos elementos internos

  // Soltar fora da área do chat também não pode abrir o arquivo no navegador.
  useEffect(() => {
    const stop = (e: globalThis.DragEvent) => { if (hasFiles(e)) e.preventDefault(); };
    window.addEventListener("dragover", stop);
    window.addEventListener("drop", stop);
    return () => { window.removeEventListener("dragover", stop); window.removeEventListener("drop", stop); };
  }, []);

  const handlers = {
    onDragEnter: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      depth.current++;
      setDragging(true);
    },
    onDragOver: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = blocked ? "none" : "copy";
    },
    onDragLeave: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth.current = Math.max(0, depth.current - 1);
      if (!depth.current) setDragging(false);
    },
    onDrop: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      depth.current = 0;
      setDragging(false);
      if (blocked) return;
      const items = Array.from(e.dataTransfer.items ?? []);
      if (items.some((i) => i.kind === "file" && i.webkitGetAsEntry?.()?.isDirectory)) {
        return onFiles([], "Pastas não podem ser enviadas. Arraste os arquivos.");
      }
      const files = Array.from(e.dataTransfer.files);
      const empty = files.find((f) => f.size === 0);
      if (empty) return onFiles([], `O arquivo ${empty.name} está vazio.`);
      if (files.length) onFiles(files);
    },
  };

  return { dragging, handlers };
}
