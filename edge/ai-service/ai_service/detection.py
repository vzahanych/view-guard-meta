"""
Detection Logic for Object Detection.

Handles person detection, vehicle detection, and custom detection classes.
"""

import logging
from typing import List, Set, Optional
from dataclasses import dataclass

from ai_service.inference import BoundingBox, DetectionResult

logger = logging.getLogger(__name__)


@dataclass
class DetectionFilter:
    """Filter configuration for detections."""
    enabled_classes: Optional[Set[str]] = None  # None = all classes
    min_confidence: float = 0.5
    min_area: float = 0.0  # Minimum bounding box area
    max_area: float = float('inf')  # Maximum bounding box area


class DetectionLogic:
    """
    Detection logic for filtering and categorizing detections.
    
    Handles person detection, vehicle detection, and custom classes.
    """
    
    # COCO class IDs for common objects
    PERSON_CLASS_ID = 0
    BICYCLE_CLASS_ID = 1
    CAR_CLASS_ID = 2
    MOTORCYCLE_CLASS_ID = 3
    AIRPLANE_CLASS_ID = 4
    BUS_CLASS_ID = 5
    TRAIN_CLASS_ID = 6
    TRUCK_CLASS_ID = 7
    BOAT_CLASS_ID = 8
    
    # Vehicle class IDs
    VEHICLE_CLASS_IDS = {
        CAR_CLASS_ID,
        MOTORCYCLE_CLASS_ID,
        BUS_CLASS_ID,
        TRUCK_CLASS_ID,
        BICYCLE_CLASS_ID,
    }
    
    def __init__(self, detection_filter: Optional[DetectionFilter] = None):
        """
        Initialize detection logic.
        
        Args:
            detection_filter: Optional filter configuration
        """
        self.filter = detection_filter or DetectionFilter()
    
    def filter_detections(self, result: DetectionResult) -> DetectionResult:
        """
        Filter detections based on configuration.
        
        Args:
            result: Detection result to filter
        
        Returns:
            Filtered detection result
        """
        filtered_boxes = []
        
        for box in result.bounding_boxes:
            # Check confidence threshold
            if box.confidence < self.filter.min_confidence:
                continue
            
            # Check class filter
            if self.filter.enabled_classes is not None:
                if box.class_name not in self.filter.enabled_classes:
                    continue
            
            # Check area constraints
            area = (box.x2 - box.x1) * (box.y2 - box.y1)
            if area < self.filter.min_area or area > self.filter.max_area:
                continue
            
            filtered_boxes.append(box)
        
        return DetectionResult(
            bounding_boxes=filtered_boxes,
            inference_time_ms=result.inference_time_ms,
            frame_shape=result.frame_shape,
            model_input_shape=result.model_input_shape,
        )
    
    def detect_persons(self, result: DetectionResult) -> List[BoundingBox]:
        """
        Extract person detections from result.
        
        Args:
            result: Detection result
        
        Returns:
            List of person bounding boxes
        """
        persons = []
        for box in result.bounding_boxes:
            if box.class_id == self.PERSON_CLASS_ID:
                persons.append(box)
        return persons
    
    def detect_vehicles(self, result: DetectionResult) -> List[BoundingBox]:
        """
        Extract vehicle detections from result.
        
        Args:
            result: Detection result
        
        Returns:
            List of vehicle bounding boxes
        """
        vehicles = []
        for box in result.bounding_boxes:
            if box.class_id in self.VEHICLE_CLASS_IDS:
                vehicles.append(box)
        return vehicles
    
    def detect_custom_classes(
        self,
        result: DetectionResult,
        class_names: List[str],
    ) -> List[BoundingBox]:
        """
        Extract detections for custom class names.
        
        Args:
            result: Detection result
            class_names: List of class names to filter
        
        Returns:
            List of bounding boxes for specified classes
        """
        class_set = set(class_names)
        detections = []
        
        for box in result.bounding_boxes:
            if box.class_name in class_set:
                detections.append(box)
        
        return detections
    
    def has_person(self, result: DetectionResult) -> bool:
        """
        Check if result contains person detections.
        
        Args:
            result: Detection result
        
        Returns:
            True if persons detected, False otherwise
        """
        return len(self.detect_persons(result)) > 0
    
    def has_vehicle(self, result: DetectionResult) -> bool:
        """
        Check if result contains vehicle detections.
        
        Args:
            result: Detection result
        
        Returns:
            True if vehicles detected, False otherwise
        """
        return len(self.detect_vehicles(result)) > 0
    
    def get_detection_summary(self, result: DetectionResult) -> dict:
        """
        Get summary of detections by class.
        
        Args:
            result: Detection result
        
        Returns:
            Dictionary with detection counts by class
        """
        summary = {}
        for box in result.bounding_boxes:
            class_name = box.class_name
            if class_name not in summary:
                summary[class_name] = {
                    "count": 0,
                    "max_confidence": 0.0,
                    "avg_confidence": 0.0,
                }
            
            summary[class_name]["count"] += 1
            summary[class_name]["max_confidence"] = max(
                summary[class_name]["max_confidence"],
                box.confidence,
            )
            summary[class_name]["avg_confidence"] = (
                summary[class_name]["avg_confidence"] * (summary[class_name]["count"] - 1) +
                box.confidence
            ) / summary[class_name]["count"]
        
        return summary
    
    def set_confidence_threshold(self, threshold: float):
        """
        Update confidence threshold.
        
        Args:
            threshold: New confidence threshold (0.0 to 1.0)
        """
        if not (0.0 <= threshold <= 1.0):
            raise ValueError(f"Confidence threshold must be between 0.0 and 1.0, got {threshold}")
        
        self.filter.min_confidence = threshold
        logger.info("Confidence threshold updated", extra={"threshold": threshold})
    
    def set_enabled_classes(self, class_names: Optional[List[str]]):
        """
        Set enabled detection classes.
        
        Args:
            class_names: List of class names to enable, or None for all classes
        """
        if class_names is not None:
            self.filter.enabled_classes = set(class_names)
        else:
            self.filter.enabled_classes = None
        
        logger.info(
            "Enabled classes updated",
            extra={"classes": class_names, "count": len(class_names) if class_names else "all"},
        )

