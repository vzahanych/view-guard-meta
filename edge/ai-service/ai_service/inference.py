"""
Inference Service for Object Detection.

Handles frame preprocessing, OpenVINO inference, and post-processing for YOLOv8 models.
"""

import logging
import time
from dataclasses import dataclass
from typing import List, Optional, Tuple
import numpy as np

logger = logging.getLogger(__name__)

# Try to import OpenVINO
try:
    import openvino.runtime as ov
    OPENVINO_AVAILABLE = True
except ImportError:
    OPENVINO_AVAILABLE = False
    logger.warning("OpenVINO not available. Install with: pip install openvino")

# Try to import OpenCV
try:
    import cv2
    CV2_AVAILABLE = True
except ImportError:
    CV2_AVAILABLE = False
    logger.warning("OpenCV not available. Install with: pip install opencv-python")


@dataclass
class BoundingBox:
    """Bounding box for detected object."""
    x1: float  # Left
    y1: float  # Top
    x2: float  # Right
    y2: float  # Bottom
    confidence: float
    class_id: int
    class_name: str


@dataclass
class DetectionResult:
    """Detection result for a frame."""
    bounding_boxes: List[BoundingBox]
    inference_time_ms: float
    frame_shape: Tuple[int, int]  # (height, width)
    model_input_shape: Tuple[int, int]  # (height, width)


class FramePreprocessor:
    """
    Frame preprocessor for YOLO models.
    
    Handles resizing, normalization, and format conversion.
    """
    
    def __init__(self, target_size: Tuple[int, int] = (640, 640)):
        """
        Initialize frame preprocessor.
        
        Args:
            target_size: Target size (width, height) for model input
        """
        self.target_size = target_size
        self.target_width, self.target_height = target_size
    
    def preprocess(self, frame: np.ndarray) -> Tuple[np.ndarray, float, Tuple[float, float]]:
        """
        Preprocess frame for YOLO inference.
        
        Args:
            frame: Input frame as numpy array (BGR format from OpenCV)
        
        Returns:
            Tuple of (preprocessed_frame, scale_factor, (pad_x, pad_y))
        """
        if not CV2_AVAILABLE:
            raise RuntimeError("OpenCV not available for preprocessing")
        
        original_height, original_width = frame.shape[:2]
        
        # Calculate scale to fit frame into target size while maintaining aspect ratio
        scale = min(
            self.target_width / original_width,
            self.target_height / original_height,
        )
        
        new_width = int(original_width * scale)
        new_height = int(original_height * scale)
        
        # Resize frame
        resized = cv2.resize(frame, (new_width, new_height), interpolation=cv2.INTER_LINEAR)
        
        # Create padded image
        padded = np.full(
            (self.target_height, self.target_width, 3),
            114,  # Gray padding (YOLO standard)
            dtype=np.uint8,
        )
        
        # Calculate padding offsets (center the image)
        pad_x = (self.target_width - new_width) // 2
        pad_y = (self.target_height - new_height) // 2
        
        # Place resized image in center
        padded[pad_y:pad_y + new_height, pad_x:pad_x + new_width] = resized
        
        # Convert BGR to RGB
        rgb_frame = cv2.cvtColor(padded, cv2.COLOR_BGR2RGB)
        
        # Normalize to [0, 1] and convert to float32
        normalized = rgb_frame.astype(np.float32) / 255.0
        
        # Convert to NCHW format (batch, channels, height, width)
        nchw = np.transpose(normalized, (2, 0, 1))
        
        # Add batch dimension
        batch = np.expand_dims(nchw, axis=0)
        
        return batch, scale, (pad_x, pad_y)
    
    def preprocess_batch(self, frames: List[np.ndarray]) -> Tuple[np.ndarray, List[float], List[Tuple[float, float]]]:
        """
        Preprocess multiple frames.
        
        Args:
            frames: List of input frames
        
        Returns:
            Tuple of (batched_preprocessed_frames, scale_factors, padding_offsets)
        """
        preprocessed = []
        scales = []
        paddings = []
        
        for frame in frames:
            preproc, scale, padding = self.preprocess(frame)
            preprocessed.append(preproc)
            scales.append(scale)
            paddings.append(padding)
        
        # Stack into batch
        batch = np.concatenate(preprocessed, axis=0)
        
        return batch, scales, paddings


class PostProcessor:
    """
    Post-processor for YOLO detection results.
    
    Handles NMS, confidence filtering, and bounding box extraction.
    """
    
    def __init__(
        self,
        confidence_threshold: float = 0.5,
        nms_threshold: float = 0.4,
        class_names: Optional[List[str]] = None,
    ):
        """
        Initialize post-processor.
        
        Args:
            confidence_threshold: Minimum confidence for detections
            nms_threshold: Non-maximum suppression threshold
            class_names: List of class names (default: COCO classes)
        """
        self.confidence_threshold = confidence_threshold
        self.nms_threshold = nms_threshold
        self.class_names = class_names or self._get_coco_class_names()
    
    def _get_coco_class_names(self) -> List[str]:
        """Get COCO class names (YOLOv8 default)."""
        return [
            "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck",
            "boat", "traffic light", "fire hydrant", "stop sign", "parking meter", "bench",
            "bird", "cat", "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra",
            "giraffe", "backpack", "umbrella", "handbag", "tie", "suitcase", "frisbee",
            "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
            "skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork",
            "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange", "broccoli",
            "carrot", "hot dog", "pizza", "donut", "cake", "chair", "couch", "potted plant",
            "bed", "dining table", "toilet", "tv", "laptop", "mouse", "remote", "keyboard",
            "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator", "book",
            "clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
        ]
    
    def process_output(
        self,
        output: np.ndarray,
        scale: float,
        padding: Tuple[float, float],
        original_shape: Tuple[int, int],
    ) -> List[BoundingBox]:
        """
        Process model output to extract bounding boxes.
        
        Args:
            output: Model output tensor (shape: [1, num_detections, 84] for YOLOv8)
            scale: Scale factor used in preprocessing
            padding: Padding offsets (pad_x, pad_y)
            original_shape: Original frame shape (height, width)
        
        Returns:
            List of BoundingBox objects
        """
        if not CV2_AVAILABLE:
            raise RuntimeError("OpenCV not available for post-processing")
        
        pad_x, pad_y = padding
        original_height, original_width = original_shape
        
        # YOLOv8 output format: [batch, num_detections, 84]
        # Each detection: [x_center, y_center, width, height, conf_class_0, conf_class_1, ...]
        # We need to reshape if needed
        if len(output.shape) == 3:
            # Remove batch dimension if present
            detections = output[0] if output.shape[0] == 1 else output
        else:
            detections = output
        
        boxes = []
        
        for detection in detections:
            # Extract box coordinates (normalized)
            x_center, y_center, width, height = detection[:4]
            
            # Extract class confidences (remaining values)
            class_scores = detection[4:]
            
            # Find class with highest confidence
            class_id = int(np.argmax(class_scores))
            confidence = float(class_scores[class_id])
            
            # Filter by confidence threshold
            if confidence < self.confidence_threshold:
                continue
            
            # Convert from normalized center format to corner format
            x1_norm = x_center - width / 2
            y1_norm = y_center - height / 2
            x2_norm = x_center + width / 2
            y2_norm = y_center + height / 2
            
            # Remove padding and scale back to original image coordinates
            x1 = (x1_norm * 640 - pad_x) / scale
            y1 = (y1_norm * 640 - pad_y) / scale
            x2 = (x2_norm * 640 - pad_x) / scale
            y2 = (y2_norm * 640 - pad_y) / scale
            
            # Clip to image boundaries
            x1 = max(0, min(x1, original_width))
            y1 = max(0, min(y1, original_height))
            x2 = max(0, min(x2, original_width))
            y2 = max(0, min(y2, original_height))
            
            # Skip invalid boxes
            if x2 <= x1 or y2 <= y1:
                continue
            
            boxes.append(
                BoundingBox(
                    x1=float(x1),
                    y1=float(y1),
                    x2=float(x2),
                    y2=float(y2),
                    confidence=confidence,
                    class_id=class_id,
                    class_name=self.class_names[class_id] if class_id < len(self.class_names) else f"class_{class_id}",
                )
            )
        
        # Apply Non-Maximum Suppression (NMS)
        boxes = self._apply_nms(boxes)
        
        return boxes
    
    def _apply_nms(self, boxes: List[BoundingBox]) -> List[BoundingBox]:
        """
        Apply Non-Maximum Suppression to remove overlapping boxes.
        
        Args:
            boxes: List of bounding boxes
        
        Returns:
            Filtered list of bounding boxes
        """
        if not boxes:
            return []
        
        if not CV2_AVAILABLE:
            # Simple NMS without OpenCV (less efficient but works)
            return self._simple_nms(boxes)
        
        # Convert to OpenCV format
        boxes_array = np.array([
            [box.x1, box.y1, box.x2 - box.x1, box.y2 - box.y1]
            for box in boxes
        ])
        scores = np.array([box.confidence for box in boxes])
        
        # Apply NMS
        indices = cv2.dnn.NMSBoxes(
            boxes_array.tolist(),
            scores.tolist(),
            self.confidence_threshold,
            self.nms_threshold,
        )
        
        if len(indices) == 0:
            return []
        
        # Filter boxes using NMS indices
        if isinstance(indices, np.ndarray):
            indices = indices.flatten()
        
        return [boxes[i] for i in indices]
    
    def _simple_nms(self, boxes: List[BoundingBox]) -> List[BoundingBox]:
        """
        Simple NMS implementation without OpenCV.
        
        Args:
            boxes: List of bounding boxes
        
        Returns:
            Filtered list of bounding boxes
        """
        if not boxes:
            return []
        
        # Sort by confidence (descending)
        sorted_boxes = sorted(boxes, key=lambda b: b.confidence, reverse=True)
        kept = []
        
        while sorted_boxes:
            # Take box with highest confidence
            current = sorted_boxes.pop(0)
            kept.append(current)
            
            # Remove boxes with high IoU overlap
            remaining = []
            for box in sorted_boxes:
                iou = self._calculate_iou(current, box)
                if iou < self.nms_threshold:
                    remaining.append(box)
            sorted_boxes = remaining
        
        return kept
    
    def _calculate_iou(self, box1: BoundingBox, box2: BoundingBox) -> float:
        """Calculate Intersection over Union (IoU) between two boxes."""
        # Calculate intersection area
        x1_inter = max(box1.x1, box2.x1)
        y1_inter = max(box1.y1, box2.y1)
        x2_inter = min(box1.x2, box2.x2)
        y2_inter = min(box1.y2, box2.y2)
        
        if x2_inter <= x1_inter or y2_inter <= y1_inter:
            return 0.0
        
        inter_area = (x2_inter - x1_inter) * (y2_inter - y1_inter)
        
        # Calculate union area
        box1_area = (box1.x2 - box1.x1) * (box1.y2 - box1.y1)
        box2_area = (box2.x2 - box2.x1) * (box2.y2 - box2.y1)
        union_area = box1_area + box2_area - inter_area
        
        if union_area == 0:
            return 0.0
        
        return inter_area / union_area


class InferenceEngine:
    """
    Inference engine for object detection.
    
    Coordinates preprocessing, inference, and post-processing.
    """
    
    def __init__(
        self,
        model_loader,
        confidence_threshold: float = 0.5,
        nms_threshold: float = 0.4,
        target_size: Tuple[int, int] = (640, 640),
    ):
        """
        Initialize inference engine.
        
        Args:
            model_loader: ModelLoader instance
            confidence_threshold: Minimum confidence for detections
            nms_threshold: NMS threshold
            target_size: Target input size for model
        """
        self.model_loader = model_loader
        self.preprocessor = FramePreprocessor(target_size=target_size)
        self.postprocessor = PostProcessor(
            confidence_threshold=confidence_threshold,
            nms_threshold=nms_threshold,
        )
        self._inference_count = 0
        self._total_inference_time = 0.0
    
    def infer(self, frame: np.ndarray) -> DetectionResult:
        """
        Perform inference on a single frame.
        
        Args:
            frame: Input frame as numpy array (BGR format)
        
        Returns:
            DetectionResult with bounding boxes and metadata
        
        Raises:
            RuntimeError: If model is not loaded or inference fails
        """
        start_time = time.time()
        
        # Get compiled model
        compiled_model = self.model_loader.get_compiled_model()
        if compiled_model is None:
            raise RuntimeError("Model not loaded. Call model_loader.load_model() first.")
        
        # Get model info
        model_info = self.model_loader.get_current_model()
        if model_info is None:
            raise RuntimeError("Model not loaded")
        
        original_shape = frame.shape[:2]  # (height, width)
        
        # Preprocess frame
        preprocessed, scale, padding = self.preprocessor.preprocess(frame)
        
        # Run inference
        # OpenVINO compiled model expects numpy array directly
        result = compiled_model([preprocessed])
        
        # Get output (assuming single output)
        output_tensor = list(result.values())[0]
        output = output_tensor.data
        
        # Post-process
        boxes = self.postprocessor.process_output(
            output=output,
            scale=scale,
            padding=padding,
            original_shape=original_shape,
        )
        
        # Calculate inference time
        inference_time = (time.time() - start_time) * 1000  # Convert to ms
        
        # Update statistics
        self._inference_count += 1
        self._total_inference_time += inference_time
        
        return DetectionResult(
            bounding_boxes=boxes,
            inference_time_ms=inference_time,
            frame_shape=original_shape,
            model_input_shape=self.preprocessor.target_size,
        )
    
    def infer_batch(self, frames: List[np.ndarray]) -> List[DetectionResult]:
        """
        Perform inference on multiple frames (batch processing).
        
        Args:
            frames: List of input frames
        
        Returns:
            List of DetectionResult objects
        """
        results = []
        for frame in frames:
            result = self.infer(frame)
            results.append(result)
        return results
    
    def get_statistics(self) -> dict:
        """
        Get inference statistics.
        
        Returns:
            Dictionary with inference statistics
        """
        avg_time = (
            self._total_inference_time / self._inference_count
            if self._inference_count > 0
            else 0.0
        )
        
        return {
            "total_inferences": self._inference_count,
            "total_time_ms": self._total_inference_time,
            "average_time_ms": avg_time,
        }
    
    def reset_statistics(self):
        """Reset inference statistics."""
        self._inference_count = 0
        self._total_inference_time = 0.0

